package main

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"go.mau.fi/whatsmeow/types"
)

// somDeTeste devolve "fala" (ruido forte) com pausas de silencio nos segundos indicados.
func somDeTeste(segundos int, silencios ...int) []float32 {
	pcm := make([]float32, segundos*taxaDoOuvido)
	for i := range pcm {
		pcm[i] = float32(math.Sin(float64(i)*0.3)) * 0.5
	}
	for _, s := range silencios {
		for i := s * taxaDoOuvido; i < (s+1)*taxaDoOuvido && i < len(pcm); i++ {
			pcm[i] = 0
		}
	}
	return pcm
}

func TestCortarEmTrechos(t *testing.T) {
	// Audio curto: um trecho so.
	if tr := cortarEmTrechos(somDeTeste(9)); len(tr) != 1 || len(tr[0]) != 9*taxaDoOuvido {
		t.Errorf("áudio curto devia ficar inteiro: %d trechos", len(tr))
	}
	// 70 segundos com silencio aos 25 s e aos 50 s: corta no silencio, nunca passa de 28 s.
	pcm := somDeTeste(70, 25, 50)
	tr := cortarEmTrechos(pcm)
	total := 0
	for i, x := range tr {
		total += len(x)
		if len(x) > trechoDoOuvido*taxaDoOuvido {
			t.Errorf("trecho %d passou de %d segundos: %.1f s", i, trechoDoOuvido, float64(len(x))/taxaDoOuvido)
		}
	}
	if total != len(pcm) {
		t.Errorf("cortar não pode perder som: %d de %d amostras", total, len(pcm))
	}
	if len(tr) != 3 {
		t.Fatalf("esperava 3 trechos, vieram %d", len(tr))
	}
	primeiro := float64(len(tr[0])) / taxaDoOuvido
	if primeiro < 25 || primeiro > 26 {
		t.Errorf("o primeiro corte devia cair no silêncio dos 25 s, caiu em %.1f s", primeiro)
	}
	// Sobra menor que 0,2 s no fim é descartada; nada de trecho vazio.
	if tr := cortarEmTrechos(make([]float32, 100)); len(tr) != 0 {
		t.Errorf("som curto demais não vira trecho: %d", len(tr))
	}
}

func TestDecodificarAudioRecusaOQueNaoEntende(t *testing.T) {
	for nome, dados := range map[string][]byte{
		"vazio":     {},
		"texto":     []byte("isto não é um áudio"),
		"ogg falso": append([]byte("OggS"), bytes.Repeat([]byte{0xff}, 200)...),
		"mp3":       append([]byte("ID3\x03\x00\x00\x00"), bytes.Repeat([]byte{0x55}, 300)...),
		"só cabeça": []byte("OggS\x00\x02\x00\x00\x00\x00\x00\x00\x00\x00"),
	} {
		pcm, _, err := decodificarOpus(bytes.NewReader(dados))
		if err == nil || !errors.Is(err, errAudioNaoEntendido) || pcm != nil {
			t.Errorf("%s: devia recusar com errAudioNaoEntendido, veio %v (%d amostras)", nome, err, len(pcm))
		}
	}
	// Arquivo de audio de verdade: só roda quando alguém aponta um (não vai áudio de pessoa no repositório).
	if caminho := os.Getenv("ATENDENTE_AUDIO_TESTE"); caminho != "" {
		pcm, cortado, err := decodificarAudio(caminho)
		if err != nil || len(pcm) < taxaDoOuvido || cortado {
			t.Fatalf("áudio de teste: %v, %d amostras", err, len(pcm))
		}
		t.Logf("áudio de teste: %.1f segundos", float64(len(pcm))/taxaDoOuvido)
	}
}

// ouvinteDeMentira faz o papel do navegador escondido: pega os trechos e devolve um texto.
func ouvinteDeMentira(t *testing.T, o *ouvido, responder func(pcm []float32) (texto, erro string)) (parar func()) {
	t.Helper()
	fim := make(chan struct{})
	pedido := func(metodo, alvo string, corpo string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(metodo, alvo, strings.NewReader(corpo))
		r.AddCookie(&http.Cookie{Name: o.nomeDoCookie(), Value: o.chave})
		if metodo == http.MethodPost {
			r.Header.Set("Content-Type", "application/json")
		}
		w := httptest.NewRecorder()
		if strings.HasPrefix(alvo, "/api/ouvido/proximo") {
			o.rotaProximo(w, r)
		} else {
			o.rotaResultado(w, r)
		}
		return w
	}
	go func() {
		for {
			select {
			case <-fim:
				return
			default:
			}
			w := pedido(http.MethodGet, "/api/ouvido/proximo", "")
			if w.Code != http.StatusOK {
				time.Sleep(10 * time.Millisecond)
				continue
			}
			bruto := w.Body.Bytes()
			pcm := make([]float32, len(bruto)/4)
			for i := range pcm {
				pcm[i] = math.Float32frombits(binary.LittleEndian.Uint32(bruto[4*i:]))
			}
			texto, erro := responder(pcm)
			corpo := `{"texto":` + strconvQuote(texto) + `}`
			if erro != "" {
				corpo = `{"erro":` + strconvQuote(erro) + `}`
			}
			pedido(http.MethodPost, "/api/ouvido/resultado?id="+w.Header().Get("X-Trecho"), corpo)
		}
	}()
	return func() { close(fim) }
}

func strconvQuote(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

// ouvidoDeTeste devolve um ouvido que se considera pronto, sem abrir navegador nenhum.
func ouvidoDeTeste(t *testing.T) *ouvido {
	t.Helper()
	o := novoOuvido()
	o.porta = 8787
	o.cmd = &exec.Cmd{} // faz de conta que o navegador escondido está aberto
	o.visto = time.Now()
	t.Setenv("ATENDENTE_NAVEGADOR", os.Args[0])
	vozDoPainel.mu.Lock()
	antes := vozDoPainel.pronto
	vozDoPainel.pronto = true
	vozDoPainel.mu.Unlock()
	t.Cleanup(func() {
		vozDoPainel.mu.Lock()
		vozDoPainel.pronto = antes
		vozDoPainel.mu.Unlock()
	})
	return o
}

func TestOuvidoTranscreveEmTrechos(t *testing.T) {
	o := ouvidoDeTeste(t)
	var mu sync.Mutex
	var tamanhos []int
	parar := ouvinteDeMentira(t, o, func(pcm []float32) (string, string) {
		mu.Lock()
		defer mu.Unlock()
		tamanhos = append(tamanhos, len(pcm))
		var soma float64
		for _, v := range pcm {
			soma += math.Abs(float64(v))
		}
		if soma < 100 {
			return "", "o som não chegou inteiro"
		}
		return map[int]string{1: " Bom dia, queria marcar um horário. ", 2: "Pode ser amanhã cedo?"}[len(tamanhos)], ""
	})
	defer parar()

	texto, err := o.transcrever(somDeTeste(40, 25))
	if err != nil || texto != "Bom dia, queria marcar um horário. Pode ser amanhã cedo?" {
		t.Fatalf("transcrição em dois trechos: %q, %v", texto, err)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(tamanhos) != 2 || tamanhos[0]+tamanhos[1] != 40*taxaDoOuvido {
		t.Errorf("o som devia chegar inteiro ao ouvinte, em 2 trechos: %v", tamanhos)
	}
}

func TestOuvidoDevolveAFalhaDoWhisper(t *testing.T) {
	o := ouvidoDeTeste(t)
	parar := ouvinteDeMentira(t, o, func(pcm []float32) (string, string) { return "", "sem memória" })
	defer parar()
	if _, err := o.transcrever(somDeTeste(3)); err == nil || !strings.Contains(err.Error(), "sem memória") {
		t.Errorf("falha do Whisper devia voltar como erro: %v", err)
	}
}

func TestOuvidoSoAtendeOProprioOuvinte(t *testing.T) {
	o := ouvidoDeTeste(t)
	// Sem a chave: nenhuma rota do ouvinte responde.
	for _, caso := range []struct {
		metodo, alvo string
		rota         http.HandlerFunc
	}{
		{"GET", "/ouvinte.html", o.rotaPagina},
		{"GET", "/ouvinte.html?c=chave-errada", o.rotaPagina},
		{"GET", "/api/ouvido/inicio", o.rotaInicio},
		{"GET", "/api/ouvido/proximo", o.rotaProximo},
		{"POST", "/api/ouvido/resultado?id=x", o.rotaResultado},
	} {
		w := httptest.NewRecorder()
		r := httptest.NewRequest(caso.metodo, caso.alvo, strings.NewReader(`{}`))
		r.AddCookie(&http.Cookie{Name: o.nomeDoCookie(), Value: "chave-errada"})
		caso.rota(w, r)
		if w.Code != http.StatusNotFound {
			t.Errorf("%s %s sem a chave devia dar 404, deu %d", caso.metodo, caso.alvo, w.Code)
		}
	}
	// Com a chave certa a página entrega o cookie, que a página não consegue ler.
	w := httptest.NewRecorder()
	o.rotaPagina(w, httptest.NewRequest("GET", "/ouvinte.html?c="+o.chave, nil))
	cookies := w.Result().Cookies()
	if w.Code != http.StatusOK || len(cookies) != 1 || !cookies[0].HttpOnly || cookies[0].SameSite != http.SameSiteStrictMode || cookies[0].Value != o.chave {
		t.Errorf("página do ouvinte: %d, cookies %+v", w.Code, cookies)
	}
	if strings.Contains(w.Body.String(), o.chave) {
		t.Errorf("a chave não pode aparecer dentro da página")
	}
	// Os arquivos da voz aceitam o ouvinte, mas não um visitante qualquer.
	antes := ouvidoDoPainel
	ouvidoDoPainel = o
	defer func() { ouvidoDoPainel = antes }()
	w = httptest.NewRecorder()
	vozDoPainel.rotaArquivo(w, httptest.NewRequest("GET", "/voz/"+vozBiblioteca, nil))
	if w.Code != http.StatusUnauthorized {
		t.Errorf("arquivo da voz sem login e sem chave devia dar 401, deu %d", w.Code)
	}
	r := httptest.NewRequest("GET", "/voz/"+vozBiblioteca, nil)
	r.AddCookie(cookies[0])
	w = httptest.NewRecorder()
	vozDoPainel.rotaArquivo(w, r)
	if w.Code == http.StatusUnauthorized {
		t.Errorf("o ouvinte precisa conseguir ler os arquivos da voz")
	}
	// Sem navegador ou sem a voz baixada, o ouvido diz que não está disponível.
	t.Setenv("ATENDENTE_NAVEGADOR", "/caminho/que/nao/existe")
	if o.disponivel() {
		t.Errorf("sem navegador o ouvido não está disponível")
	}
	if _, err := o.transcrever(somDeTeste(2)); !errors.Is(err, errSemOuvido) {
		t.Errorf("sem ouvido, transcrever devia dizer isso: %v", err)
	}
}

func TestAudioTranscritoVaiMarcadoParaAIA(t *testing.T) {
	audio := Msg{De: "cliente", Tipo: "audio", Midia: "f1.ogg", Texto: "Queria marcar um horário amanhã."}
	if got := textoParaIA(audio); got != "[áudio do cliente, transcrito] Queria marcar um horário amanhã." {
		t.Errorf("áudio transcrito: %q", got)
	}
	if got := textoParaIA(Msg{De: "cliente", Tipo: "audio", Midia: "f2.ogg"}); got != "[áudio]" {
		t.Errorf("áudio sem transcrição: %q", got)
	}
	if got := textoParaIA(Msg{De: "cliente", Texto: "oi"}); got != "oi" {
		t.Errorf("texto comum: %q", got)
	}
	entrada := montarEntrada([]Msg{audio, {De: "atendente", Texto: "Claro!"}}, []string{textoParaIA(Msg{De: "cliente", Tipo: "audio", Texto: "Às nove."})}, nil, nil)
	if !strings.Contains(entrada, "Cliente: [áudio do cliente, transcrito] Queria marcar um horário amanhã.\n") || !strings.Contains(entrada, "Cliente: [áudio do cliente, transcrito] Às nove.\n") {
		t.Errorf("a conversa da IA devia marcar os áudios:\n%s", entrada)
	}
	if !strings.Contains(formatoSaida, "[áudio do cliente, transcrito]") {
		t.Errorf("as instruções precisam explicar o que é um áudio transcrito")
	}
	if limparTranscricao("  Legendas pela comunidade Amara.org ") != "" || limparTranscricao(" Oi,   tudo bem? ") != "Oi, tudo bem?" {
		t.Errorf("limpeza da transcrição")
	}
}

func TestEntrarNaVezRespeitaOLimite(t *testing.T) {
	a := novoApp()
	cfg := Config{LimiteMsgs: 2}
	agora := time.Now()
	r1, iniciar1, _ := a.entrarNaVez("x", cfg, agora)
	r2, iniciar2, _ := a.entrarNaVez("x", cfg, agora)
	r3, _, aviso3 := a.entrarNaVez("x", cfg, agora)
	r4, _, aviso4 := a.entrarNaVez("x", cfg, agora)
	if !r1 || !iniciar1 || !r2 || iniciar2 || r3 || !aviso3 || r4 || aviso4 {
		t.Errorf("limite por contato: %v %v | %v %v | %v %v | %v %v", r1, iniciar1, r2, iniciar2, r3, aviso3, r4, aviso4)
	}
}

// Áudio que não dá para ouvir (arquivo ainda não baixado) com o atendente pausado: não
// trava, não manda nada e não inventa texto.
func TestOuvirMensagemSemArquivoNaoQuebra(t *testing.T) {
	a := novoApp()
	a.leads["x"] = &Lead{ID: "x", Telefone: "5527999990001", Historico: []Msg{{De: "cliente", Tipo: "audio", ID: "m1"}}}
	info := types.MessageInfo{ID: "m1"}
	a.ouvirMensagem(nil, info, types.JID{}, "x")
	if m := a.leads["x"].Historico[0]; m.Texto != "" || m.Pendente || a.leads["x"].AvisouMidia {
		t.Errorf("sem arquivo e com o atendente pausado, nada muda: %+v", m)
	}
}

// Em computador onde o navegador escondido não roda, o ouvido desiste rápido e fica de
// lado: os áudios seguintes recebem o aviso de "só leio texto" na hora, sem esperar.
func TestOuvidoFicaDeLadoDepoisDeFalharDuasVezes(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("usa um programa do Unix que sai na hora")
	}
	falso, err := exec.LookPath("false")
	if err != nil {
		t.Skip("sem o programa false")
	}
	o := ouvidoDeTeste(t)
	o.cmd = nil // nenhum navegador aberto: vai tentar abrir
	o.perfil = t.TempDir()
	t.Setenv("ATENDENTE_NAVEGADOR", falso)
	som := make([]float32, taxaDoOuvido)
	for i := 0; i < 2; i++ {
		inicio := time.Now()
		if _, err := o.transcrever(som); err == nil {
			t.Fatalf("tentativa %d: navegador que fecha na hora não transcreve", i+1)
		}
		if d := time.Since(inicio); d > 10*time.Second {
			t.Fatalf("tentativa %d: demorou %v para desistir de um navegador que fechou na hora", i+1, d)
		}
	}
	if o.disponivel() {
		t.Error("depois de duas falhas seguidas, o ouvido tem que ficar de lado por um tempo")
	}
	if _, err := o.transcrever(som); !errors.Is(err, errSemOuvido) {
		t.Errorf("com o ouvido de lado, a resposta é imediata: %v", err)
	}
}

// Muitos áudios de uma vez: os que passam do limite voltam na hora, sem transcrição.
func TestOuvidoLimitaAFila(t *testing.T) {
	o := ouvidoDeTeste(t)
	o.naFila.Store(maxAudiosNaFila)
	if _, err := o.transcrever(make([]float32, taxaDoOuvido)); err == nil {
		t.Fatal("fila cheia tem que recusar o áudio")
	}
	if n := o.naFila.Load(); n != maxAudiosNaFila {
		t.Errorf("a conta da fila tem que voltar ao que era: %d", n)
	}
}

// Ponta a ponta, com o painel, o navegador escondido e o Whisper de verdade. Só roda
// quando alguém aponta um áudio do WhatsApp (ATENDENTE_AUDIO_TESTE) e a pasta dados/voz
// já baixada (ATENDENTE_VOZ_TESTE). ATENDENTE_AUDIO_ESPERA, se vier, é um pedaço do que
// a transcrição tem que conter. Não vai áudio de ninguém no repositório.
func TestOuvidoDePontaAPonta(t *testing.T) {
	audio, voz := os.Getenv("ATENDENTE_AUDIO_TESTE"), os.Getenv("ATENDENTE_VOZ_TESTE")
	if audio == "" || voz == "" {
		t.Skip("só roda com ATENDENTE_AUDIO_TESTE e ATENDENTE_VOZ_TESTE")
	}
	if acharNavegador() == "" {
		t.Skip("sem Chrome, Edge, Brave ou Chromium neste computador")
	}
	som, err := os.ReadFile(audio)
	if err != nil {
		t.Fatal(err)
	}
	pastaDeTeste(t)
	if err := os.MkdirAll(pastaMidia, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(voz, filepath.Join(pastaDados, "voz")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pastaMidia, "f1.ogg"), som, 0o600); err != nil {
		t.Fatal(err)
	}
	if vozDoPainel.comecar() {
		vozDoPainel.executar() // confere os arquivos da voz, como o programa faz ao ligar
	}
	perfil, err := os.MkdirTemp("", "atendente-ouvinte-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ouvidoDoPainel.mu.Lock()
		ouvidoDoPainel.encerrar()
		ouvidoDoPainel.porta, ouvidoDoPainel.perfil = 0, ""
		ouvidoDoPainel.mu.Unlock()
		vozDoPainel.mu.Lock()
		vozDoPainel.pronto = false
		vozDoPainel.mu.Unlock()
		time.Sleep(500 * time.Millisecond)
		_ = os.RemoveAll(perfil)
	})

	a := novoApp()
	t.Setenv("ATENDENTE_PORTA", "18795")
	if _, err := a.iniciarPainel(); err != nil {
		t.Fatal(err)
	}
	ouvidoDoPainel.mu.Lock()
	ouvidoDoPainel.perfil = perfil
	ouvidoDoPainel.mu.Unlock()
	if !ouvidoDoPainel.disponivel() {
		t.Fatalf("o ouvido tinha que estar disponível: voz %v", vozDoPainel.estado())
	}

	// O atendente está pausado (instalação nova): o áudio é transcrito e guardado para a
	// equipe ler, mas ninguém responde.
	a.leads["x"] = &Lead{ID: "x", Telefone: "5527999990001", Historico: []Msg{{De: "cliente", Tipo: "audio", ID: "m1", Midia: "f1.ogg", Hora: time.Now()}}}
	inicio := time.Now()
	a.ouvirMensagem(nil, types.MessageInfo{ID: "m1"}, types.JID{}, "x")
	m := a.leads["x"].Historico[0]
	if strings.TrimSpace(m.Texto) == "" {
		t.Fatalf("o áudio não foi transcrito: %+v", m)
	}
	t.Logf("transcrito em %.1f s: %q", time.Since(inicio).Seconds(), m.Texto)
	if espera := os.Getenv("ATENDENTE_AUDIO_ESPERA"); espera != "" && !strings.Contains(chaveTexto(m.Texto), chaveTexto(espera)) {
		t.Errorf("a transcrição %q não contém %q", m.Texto, espera)
	}
	if m.Pendente || a.leads["x"].AvisouMidia {
		t.Errorf("com o atendente pausado, a transcrição fica guardada e ninguém responde: %+v", m)
	}
	if got := textoParaIA(m); !strings.HasPrefix(got, cabecalhoDeAudio) {
		t.Errorf("a IA tem que saber que o texto veio de um áudio: %q", got)
	}

	// Segundo áudio: o navegador escondido já está aberto, então é mais rápido.
	a.leads["x"].Historico = append(a.leads["x"].Historico, Msg{De: "cliente", Tipo: "audio", ID: "m2", Midia: "f1.ogg", Hora: time.Now()})
	inicio = time.Now()
	a.ouvirMensagem(nil, types.MessageInfo{ID: "m2"}, types.JID{}, "x")
	if m2 := a.leads["x"].Historico[1]; strings.TrimSpace(m2.Texto) == "" {
		t.Errorf("o segundo áudio não foi transcrito: %+v", m2)
	}
	t.Logf("segundo áudio transcrito em %.1f s", time.Since(inicio).Seconds())
}
