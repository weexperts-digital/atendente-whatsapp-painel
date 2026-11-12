package main

import (
	"bufio"
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"math"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/pion/opus"
	"github.com/pion/opus/pkg/oggreader"
)

// Ouvido: o atendente entende os audios que os clientes mandam no WhatsApp.
//
// Como funciona:
//  1. O audio do WhatsApp (Opus dentro de Ogg) e decodificado aqui, em Go puro, para som
//     cru de 16 kHz. O arquivo do cliente nunca e aberto por outro programa.
//  2. A transcricao usa o mesmo Whisper da voz do painel (dados/voz), que roda dentro de
//     um navegador. O programa abre, sem janela, o navegador que ja existe no computador
//     (Chrome, Edge, Brave ou Chromium) em uma pagina so dele, o "ouvinte".
//  3. O ouvinte pede trabalho ao programa, transcreve e devolve o texto.
//
// Nada e baixado a mais, nao ha chave e o audio nao sai do computador. O texto que volta
// e um dado, como qualquer mensagem de cliente: nunca uma instrucao.

const (
	taxaDoOuvido      = 16000            // amostras por segundo que o Whisper espera
	maxSegundosAudio  = 5 * 60           // audio maior que isso e ouvido ate aqui
	trechoDoOuvido    = 28               // segundos por trecho entregue ao Whisper
	ociosoDoOuvido    = 8 * time.Minute  // sem audio por esse tempo, o navegador escondido fecha
	esperaDoOuvinte   = 90 * time.Second // tempo para o navegador abrir e carregar o Whisper
	sinalDoOuvinte    = 45 * time.Second // sem pedir trabalho por esse tempo, o ouvinte e dado como morto
	esperaLongaOuvido = 20 * time.Second // quanto o ouvinte espera por trabalho em cada pedido
	maxAudiosNaFila   = 8                // audios esperando a vez; passou disso, vale o aviso de "so leio texto"
	pausaDoOuvido     = 15 * time.Minute // descanso depois de o navegador escondido falhar duas vezes seguidas
)

var (
	errAudioNaoEntendido = errors.New("formato de áudio que o ouvido não entende")
	errSemOuvido         = errors.New("o ouvido não está disponível neste computador")
)

// ---------- 1. do arquivo do WhatsApp para som cru ----------

// decodificarAudio le um audio Ogg/Opus e devolve o som em 16 kHz, um canal, e se o
// audio foi cortado por passar do limite.
func decodificarAudio(caminho string) (pcm []float32, cortado bool, err error) {
	f, err := os.Open(caminho)
	if err != nil {
		return nil, false, err
	}
	defer f.Close()
	return decodificarOpus(bufio.NewReader(f))
}

func decodificarOpus(r io.Reader) (pcm []float32, cortado bool, err error) {
	// O decodificador e codigo de terceiros lendo um arquivo que veio de fora: se ele
	// tropecar em um arquivo malfeito, o atendente segue de pe.
	defer func() {
		if p := recover(); p != nil {
			pcm, cortado, err = nil, false, fmt.Errorf("%w: %v", errAudioNaoEntendido, p)
		}
	}()
	ogg, _, err := oggreader.NewWith(r)
	if err != nil {
		return nil, false, fmt.Errorf("%w: %v", errAudioNaoEntendido, err)
	}
	dec, err := opus.NewDecoderWithOutput(taxaDoOuvido, 1)
	if err != nil {
		return nil, false, err
	}
	quadro := make([]float32, taxaDoOuvido*120/1000*2) // um pacote Opus tem no maximo 120 ms
	limite := maxSegundosAudio * taxaDoOuvido
	pacotes, falhas := 0, 0
	for {
		segmentos, _, err := ogg.ParseNextPage()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			if pacotes == 0 {
				return nil, false, fmt.Errorf("%w: %v", errAudioNaoEntendido, err)
			}
			break // arquivo cortado no fim: fica o que deu para ouvir
		}
		for _, s := range segmentos {
			if len(s) >= 8 && (string(s[:8]) == "OpusHead" || string(s[:8]) == "OpusTags") {
				continue
			}
			pacotes++
			n, err := dec.DecodeToFloat32(s, quadro)
			if err != nil || n <= 0 || n > len(quadro) {
				falhas++
				continue
			}
			pcm = append(pcm, quadro[:n]...)
			if len(pcm) >= limite {
				return pcm[:limite], true, nil
			}
		}
	}
	if pacotes == 0 || len(pcm) < taxaDoOuvido/5 || falhas*2 > pacotes {
		return nil, false, fmt.Errorf("%w: %d de %d pacotes com falha", errAudioNaoEntendido, falhas, pacotes)
	}
	return pcm, false, nil
}

// cortarEmTrechos divide o som em pedacos de ate 28 segundos, cortando no ponto mais
// silencioso dos ultimos segundos de cada pedaco, para nao partir uma palavra ao meio.
func cortarEmTrechos(pcm []float32) [][]float32 {
	maximo := trechoDoOuvido * taxaDoOuvido
	busca := 8 * taxaDoOuvido // procura o silencio nos ultimos 8 segundos
	janela := taxaDoOuvido / 5
	var trechos [][]float32
	for len(pcm) > maximo {
		corte, menor := maximo, math.MaxFloat64
		for i := maximo - busca; i+janela <= maximo; i += janela / 2 {
			var energia float64
			for _, v := range pcm[i : i+janela] {
				energia += float64(v) * float64(v)
			}
			if energia < menor {
				menor, corte = energia, i+janela/2
			}
		}
		trechos = append(trechos, pcm[:corte])
		pcm = pcm[corte:]
	}
	if len(pcm) >= taxaDoOuvido/5 {
		trechos = append(trechos, pcm)
	}
	return trechos
}

// ---------- 2. a fila entre o programa e o ouvinte ----------

type trechoParaOuvir struct {
	id       string
	pcm      []float32
	resposta chan respostaDoOuvinte
}

type respostaDoOuvinte struct {
	texto string
	err   error
}

type ouvido struct {
	mu       sync.Mutex
	chave    string // segredo do ouvinte: so quem tem a chave pega audio e devolve texto
	fila     chan *trechoParaOuvir
	enviados map[string]*trechoParaOuvir
	cmd      *exec.Cmd
	visto    time.Time // ultima vez que o ouvinte pediu trabalho
	usado    time.Time // ultimo audio transcrito
	abrindo  bool
	porta    int
	perfil   string     // pasta do navegador escondido (vazio = dados/ouvinte)
	uso      sync.Mutex // um audio por vez
	saiu     bool       // o navegador escondido fechou sozinho antes de ficar pronto
	falhas   int        // vezes seguidas em que o navegador escondido nao ficou pronto
	pausaAte time.Time  // ate quando o ouvido fica de lado depois de falhar
	naFila   atomic.Int32
}

var ouvidoDoPainel = novoOuvido()

func novoOuvido() *ouvido {
	b := make([]byte, 24)
	_, _ = rand.Read(b)
	return &ouvido{chave: hex.EncodeToString(b), fila: make(chan *trechoParaOuvir, 64), enviados: map[string]*trechoParaOuvir{}}
}

func (o *ouvido) nomeDoCookie() string { return "ouvinte_" + strconv.Itoa(o.porta) }

// autorizado diz se o pedido vem do ouvinte que este programa abriu.
func (o *ouvido) autorizado(r *http.Request) bool {
	c, err := r.Cookie(o.nomeDoCookie())
	return err == nil && subtle.ConstantTimeCompare([]byte(c.Value), []byte(o.chave)) == 1
}

// ---------- 3. o navegador escondido ----------

// acharNavegador procura um navegador da familia do Chrome ja instalado. Vale primeiro o
// que a pessoa indicar em ATENDENTE_NAVEGADOR.
func acharNavegador() string {
	if v := strings.TrimSpace(os.Getenv("ATENDENTE_NAVEGADOR")); v != "" {
		if existeArquivo(v) {
			return v
		}
		if p, err := exec.LookPath(v); err == nil {
			return p
		}
		return ""
	}
	var candidatos []string
	casa, _ := os.UserHomeDir()
	switch runtime.GOOS {
	case "darwin":
		for _, raiz := range []string{"/Applications", filepath.Join(casa, "Applications")} {
			candidatos = append(candidatos,
				filepath.Join(raiz, "Google Chrome.app/Contents/MacOS/Google Chrome"),
				filepath.Join(raiz, "Microsoft Edge.app/Contents/MacOS/Microsoft Edge"),
				filepath.Join(raiz, "Brave Browser.app/Contents/MacOS/Brave Browser"),
				filepath.Join(raiz, "Chromium.app/Contents/MacOS/Chromium"))
		}
	case "windows":
		for _, raiz := range []string{os.Getenv("ProgramFiles"), os.Getenv("ProgramFiles(x86)"), os.Getenv("LocalAppData")} {
			if raiz == "" {
				continue
			}
			candidatos = append(candidatos,
				filepath.Join(raiz, `Google\Chrome\Application\chrome.exe`),
				filepath.Join(raiz, `Microsoft\Edge\Application\msedge.exe`),
				filepath.Join(raiz, `BraveSoftware\Brave-Browser\Application\brave.exe`),
				filepath.Join(raiz, `Chromium\Application\chrome.exe`))
		}
	default:
		for _, nome := range []string{"google-chrome", "google-chrome-stable", "chromium", "chromium-browser", "microsoft-edge", "microsoft-edge-stable", "brave-browser"} {
			if p, err := exec.LookPath(nome); err == nil {
				return p
			}
		}
	}
	for _, c := range candidatos {
		if existeArquivo(c) {
			return c
		}
	}
	return ""
}

// disponivel diz se da para ouvir audios agora: o Whisper ja esta baixado e ha um
// navegador que sirva.
func (o *ouvido) disponivel() bool {
	vozDoPainel.mu.Lock()
	pronto := vozDoPainel.pronto
	vozDoPainel.mu.Unlock()
	o.mu.Lock()
	emPausa := time.Now().Before(o.pausaAte)
	o.mu.Unlock()
	return pronto && !emPausa && o.porta > 0 && acharNavegador() != ""
}

// vivo diz se o ouvinte pediu trabalho ha pouco. Chame com o.mu travado.
func (o *ouvido) vivo() bool {
	return o.cmd != nil && time.Since(o.visto) < sinalDoOuvinte
}

// garantir deixa o ouvinte pronto: abre o navegador escondido se preciso e espera ele
// carregar o Whisper e pedir o primeiro trabalho. Se o navegador nao ficar pronto duas
// vezes seguidas (computador em que ele nao roda escondido), o ouvido fica de lado por
// um tempo e os audios voltam a receber o aviso de "so leio texto" na hora.
func (o *ouvido) garantir() error {
	err := o.abrir()
	o.mu.Lock()
	defer o.mu.Unlock()
	if err == nil {
		o.falhas = 0
		return nil
	}
	o.falhas++
	if o.falhas >= 2 {
		o.falhas = 0
		o.pausaAte = time.Now().Add(pausaDoOuvido)
		log.Printf("OUVIDO: o navegador escondido não ficou pronto duas vezes seguidas. Áudios ficam sem transcrição por %d minutos.", int(pausaDoOuvido.Minutes()))
	}
	return err
}

func (o *ouvido) abrir() error {
	o.mu.Lock()
	if o.vivo() {
		o.mu.Unlock()
		return nil
	}
	if o.abrindo {
		o.mu.Unlock()
		return o.esperarOuvinte()
	}
	o.abrindo = true
	o.encerrar() // sobra de um navegador que parou de responder
	o.mu.Unlock()
	defer func() {
		o.mu.Lock()
		o.abrindo = false
		o.mu.Unlock()
	}()

	bin := acharNavegador()
	if bin == "" || o.porta == 0 {
		return errSemOuvido
	}
	perfil := o.perfil
	if perfil == "" {
		perfil = filepath.Join(pastaDados, "ouvinte")
	}
	perfil, err := filepath.Abs(perfil)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(perfil, 0o700); err != nil {
		return err
	}
	endereco := fmt.Sprintf("http://127.0.0.1:%d/ouvinte.html?c=%s", o.porta, o.chave)
	cmd := exec.Command(bin,
		"--headless=new", "--disable-gpu", "--mute-audio", "--no-first-run", "--no-default-browser-check",
		"--disable-extensions", "--disable-sync", "--disable-background-networking", "--disable-component-update",
		"--disable-default-apps", "--metrics-recording-only", "--password-store=basic", "--use-mock-keychain",
		"--user-data-dir="+perfil, "--window-size=480,320", endereco)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = nil, nil, nil
	// O navegador escondido nunca sobra: a vida dele fica amarrada a deste programa.
	depoisDeIniciar, err := amarrarNavegador(cmd)
	if err != nil {
		depoisDeIniciar = func() {}
	}
	if err := cmd.Start(); err != nil {
		depoisDeIniciar()
		return fmt.Errorf("não consegui abrir o navegador escondido: %w", err)
	}
	depoisDeIniciar()
	o.mu.Lock()
	o.cmd = cmd
	o.visto = time.Time{}
	o.saiu = false
	o.mu.Unlock()
	go func() {
		_ = cmd.Wait() // recolhe o processo quando ele sair
		o.mu.Lock()
		if o.cmd == cmd { // fechou sozinho: quem espera por ele desiste na hora
			o.cmd = nil
			o.saiu = true
		}
		o.mu.Unlock()
	}()
	log.Printf("OUVIDO: navegador escondido aberto para transcrever áudios")
	return o.esperarOuvinte()
}

func (o *ouvido) esperarOuvinte() error {
	fim := time.Now().Add(esperaDoOuvinte)
	for time.Now().Before(fim) {
		o.mu.Lock()
		vivo, saiu := o.vivo(), o.saiu
		o.mu.Unlock()
		if vivo {
			return nil
		}
		if saiu {
			return errors.New("o navegador escondido fechou antes de ficar pronto")
		}
		time.Sleep(250 * time.Millisecond)
	}
	o.mu.Lock()
	o.encerrar()
	o.mu.Unlock()
	return errors.New("o navegador escondido não respondeu a tempo")
}

// encerrar fecha o navegador escondido. Chame com o.mu travado.
func (o *ouvido) encerrar() {
	if o.cmd != nil && o.cmd.Process != nil {
		_ = o.cmd.Process.Kill()
	}
	o.cmd = nil
	o.visto = time.Time{}
	for id, t := range o.enviados {
		t.resposta <- respostaDoOuvinte{err: errors.New("o ouvinte fechou antes de responder")}
		delete(o.enviados, id)
	}
}

// vigiar fecha o navegador escondido depois de um tempo sem audio, para devolver a
// memoria ao computador.
func (o *ouvido) vigiar() {
	for {
		time.Sleep(time.Minute)
		o.mu.Lock()
		if o.cmd != nil && !o.abrindo && time.Since(o.usado) > ociosoDoOuvido && len(o.enviados) == 0 {
			o.encerrar()
			log.Printf("OUVIDO: navegador escondido fechado por falta de uso")
		}
		o.mu.Unlock()
	}
}

// transcrever devolve o texto falado no som (16 kHz, um canal).
func (o *ouvido) transcrever(pcm []float32) (string, error) {
	if !o.disponivel() {
		return "", errSemOuvido
	}
	// Muitos audios de uma vez nao seguram o computador: os que passam do limite ficam
	// sem transcricao.
	if o.naFila.Add(1) > maxAudiosNaFila {
		o.naFila.Add(-1)
		return "", errors.New("muitos áudios esperando a vez")
	}
	defer o.naFila.Add(-1)
	o.uso.Lock()
	defer o.uso.Unlock()
	if err := o.garantir(); err != nil {
		return "", err
	}
	var partes []string
	for _, trecho := range cortarEmTrechos(pcm) {
		pedido := &trechoParaOuvir{id: novoID("t"), pcm: trecho, resposta: make(chan respostaDoOuvinte, 2)}
		segundos := time.Duration(len(trecho)/taxaDoOuvido+1) * time.Second
		select {
		case o.fila <- pedido:
		default:
			return "", errors.New("fila do ouvido cheia")
		}
		select {
		case r := <-pedido.resposta:
			if r.err != nil {
				return "", r.err
			}
			if t := strings.TrimSpace(r.texto); t != "" {
				partes = append(partes, t)
			}
		case <-time.After(60*time.Second + 6*segundos):
			o.mu.Lock()
			delete(o.enviados, pedido.id)
			o.encerrar() // travou: na proxima vez abre de novo
			o.mu.Unlock()
			return "", errors.New("o ouvido demorou demais para transcrever")
		}
	}
	o.mu.Lock()
	o.usado = time.Now()
	o.mu.Unlock()
	return limparTranscricao(strings.Join(partes, " ")), nil
}

// limparTranscricao tira o que o Whisper costuma inventar quando nao ha fala.
func limparTranscricao(s string) string {
	s = umaLinha(s)
	baixo := strings.ToLower(strings.Trim(s, " .!?,"))
	for _, vazio := range []string{"", "[música]", "[musica]", "[silêncio]", "[silencio]", "(música)", "legendas pela comunidade amara.org", "obrigado por assistir", "e aí"} {
		if baixo == vazio {
			return ""
		}
	}
	return cortar(s, 6000)
}

// ---------- 4. as rotas do ouvinte ----------

const paginaDoOuvinte = `<!doctype html>
<html lang="pt-BR"><head><meta charset="utf-8"><title>Ouvinte do atendente</title></head>
<body><script src="/ouvinte.js" defer></script></body></html>
`

// rotasDoOuvido registra as rotas que so o navegador escondido usa. Elas nao pedem login:
// pedem a chave secreta do ouvinte, sorteada a cada vez que o programa liga.
func (o *ouvido) rotas(mux *http.ServeMux) {
	mux.HandleFunc("/ouvinte.html", o.rotaPagina)
	mux.HandleFunc("/api/ouvido/inicio", o.rotaInicio)
	mux.HandleFunc("/api/ouvido/proximo", o.rotaProximo)
	mux.HandleFunc("/api/ouvido/resultado", o.rotaResultado)
}

// rotaPagina entrega a pagina do ouvinte e guarda a chave em um cookie que a pagina nao le.
func (o *ouvido) rotaPagina(w http.ResponseWriter, r *http.Request) {
	if subtle.ConstantTimeCompare([]byte(r.URL.Query().Get("c")), []byte(o.chave)) != 1 {
		http.NotFound(w, r)
		return
	}
	http.SetCookie(w, &http.Cookie{Name: o.nomeDoCookie(), Value: o.chave, Path: "/", HttpOnly: true, SameSite: http.SameSiteStrictMode})
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = io.WriteString(w, paginaDoOuvinte)
}

func (o *ouvido) rotaInicio(w http.ResponseWriter, r *http.Request) {
	if !o.autorizado(r) {
		http.NotFound(w, r)
		return
	}
	responderJSON(w, http.StatusOK, map[string]any{"arquivos": vozDoPainel.estado()["arquivos"]})
}

// rotaProximo entrega o proximo trecho de audio. Espera ate 20 segundos por trabalho.
// 204 quer dizer "nada agora, pergunte de novo". 410 quer dizer "pode fechar".
func (o *ouvido) rotaProximo(w http.ResponseWriter, r *http.Request) {
	if !o.autorizado(r) {
		http.NotFound(w, r)
		return
	}
	o.mu.Lock()
	if o.cmd == nil { // navegador que sobrou de outra vez: manda fechar
		o.mu.Unlock()
		w.WriteHeader(http.StatusGone)
		return
	}
	o.visto = time.Now()
	o.mu.Unlock()
	select {
	case t := <-o.fila:
		o.mu.Lock()
		o.enviados[t.id] = t
		o.visto = time.Now().Add(time.Duration(len(t.pcm)/taxaDoOuvido*4) * time.Second) // ocupado transcrevendo: nao e dado como morto
		o.mu.Unlock()
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("X-Trecho", t.id)
		corpo := make([]byte, 4*len(t.pcm))
		for i, v := range t.pcm {
			binary.LittleEndian.PutUint32(corpo[4*i:], math.Float32bits(v))
		}
		_, _ = w.Write(corpo)
	case <-time.After(esperaLongaOuvido):
		w.WriteHeader(http.StatusNoContent)
	case <-r.Context().Done():
	}
}

// rotaResultado recebe o texto de um trecho.
func (o *ouvido) rotaResultado(w http.ResponseWriter, r *http.Request) {
	if !o.autorizado(r) {
		http.NotFound(w, r)
		return
	}
	var p struct {
		Texto string `json:"texto"`
		Erro  string `json:"erro"`
	}
	if !lerPedido(w, r, &p) {
		return
	}
	o.mu.Lock()
	t := o.enviados[r.URL.Query().Get("id")]
	delete(o.enviados, r.URL.Query().Get("id"))
	o.visto = time.Now()
	o.mu.Unlock()
	if t == nil {
		responderOK(w, nil)
		return
	}
	if p.Erro != "" {
		t.resposta <- respostaDoOuvinte{err: errors.New("o Whisper falhou: " + cortar(umaLinha(p.Erro), 200))}
	} else {
		t.resposta <- respostaDoOuvinte{texto: p.Texto}
	}
	responderOK(w, nil)
}

// ---------- 5. ligacao com o atendimento ----------

// ouvirArquivo decodifica e transcreve um audio guardado em dados/midia.
func (a *App) ouvirArquivo(nome string) (texto string, cortado bool, err error) {
	if !strings.HasSuffix(strings.ToLower(nome), ".ogg") && !strings.HasSuffix(strings.ToLower(nome), ".opus") {
		return "", false, errAudioNaoEntendido
	}
	pcm, cortado, err := decodificarAudio(filepath.Join(pastaMidia, filepath.Base(nome)))
	if err != nil {
		return "", false, err
	}
	texto, err = ouvidoDoPainel.transcrever(pcm)
	return texto, cortado, err
}

// cabecalhoDeAudio vem antes da transcricao quando ela vai para a IA.
const cabecalhoDeAudio = "[áudio do cliente, transcrito] "

// textoParaIA e o que a IA le de uma mensagem do cliente: o texto ou, em audio, a transcricao.
func textoParaIA(m Msg) string {
	if m.Tipo == "audio" && strings.TrimSpace(m.Texto) != "" {
		return cabecalhoDeAudio + m.Texto
	}
	return m.textoPlano()
}

// ouvirTesteDeLinha e o teste de linha de comando: ./atendente -ouvir arquivo.ogg
// Sobe so o que o ouvinte precisa, em uma porta livre, transcreve e sai.
func (a *App) ouvirTesteDeLinha(caminho string) int {
	if vozDoPainel.comecar() {
		vozDoPainel.executar()
	}
	pcm, cortado, err := decodificarAudio(caminho)
	if err != nil {
		fmt.Println("FALHA ao decodificar:", err)
		return 1
	}
	fmt.Printf("ÁUDIO: %.1f segundos%s\n", float64(len(pcm))/taxaDoOuvido, map[bool]string{true: " (cortado no limite)", false: ""}[cortado])
	escuta, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		fmt.Println("FALHA:", err)
		return 1
	}
	ouvidoDoPainel.porta = escuta.Addr().(*net.TCPAddr).Port
	if pasta, err := os.MkdirTemp("", "atendente-ouvinte-"); err == nil {
		ouvidoDoPainel.perfil = pasta // perfil proprio: nao briga com o atendente que esta no ar
		defer os.RemoveAll(pasta)
	}
	mux := http.NewServeMux()
	mux.Handle("/", a.rotaArquivos())
	a.rotasDeVoz(mux)
	ouvidoDoPainel.rotas(mux)
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cabecalhosDoPainel(w)
		mux.ServeHTTP(w, r)
	})}
	go func() { _ = srv.Serve(escuta) }()
	inicio := time.Now()
	texto, err := ouvidoDoPainel.transcrever(pcm)
	ouvidoDoPainel.mu.Lock()
	ouvidoDoPainel.encerrar()
	ouvidoDoPainel.mu.Unlock()
	_ = srv.Shutdown(context.Background())
	if err != nil {
		fmt.Println("FALHA ao transcrever:", err)
		return 1
	}
	fmt.Printf("TEMPO: %.1f segundos\n", time.Since(inicio).Seconds())
	b, _ := json.Marshal(texto)
	fmt.Println("TEXTO:", string(b))
	return 0
}
