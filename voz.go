package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Reconhecimento de voz local (Whisper rodando na propria pagina, sem chave e sem
// mandar o audio para fora do computador).
//
// A pagina usa tres coisas de terceiros, que NAO ficam no repositorio:
//   - a biblioteca transformers.js (Hugging Face), licenca Apache-2.0;
//   - o ONNX Runtime Web (o motor em WebAssembly), licenca MIT;
//   - o modelo Whisper small ja convertido e quantizado (Xenova/whisper-small), Apache-2.0.
//
// Este arquivo baixa tudo para dados/voz na primeira vez, de enderecos HTTPS fixos
// (versao exata), confere o SHA-256 de cada arquivo com a lista abaixo e so entao
// serve em /voz/, na mesma origem da pagina. A pagina nunca busca nada de fora.

// arquivoDeVoz e um arquivo de terceiros de que a voz precisa.
type arquivoDeVoz struct {
	Caminho string // onde fica dentro de dados/voz e de /voz/ (sempre com barra normal)
	Origem  string // endereco fixo de onde baixar
	SHA256  string // o que o arquivo tem de ser, byte a byte
	Tamanho int64
}

// Versoes exatas. Para trocar de versao: mude os enderecos, os caminhos (eles levam a
// versao no nome, por causa do cache longo do navegador) e a lista de SHA-256.
const (
	origemTransformers = "https://cdn.jsdelivr.net/npm/@huggingface/transformers@4.3.2/dist/"
	origemOnnx         = "https://cdn.jsdelivr.net/npm/onnxruntime-web@1.31.0-dev.20260914-8d85527a0/dist/"
	origemWhisper      = "https://huggingface.co/Xenova/whisper-small/resolve/2d67713f236afa48a18992566e7647f6ca848e13/"

	vozBiblioteca = "transformers-4.3.2/transformers.min.js"
	vozPastaOnnx  = "onnxruntime-web-1.31.0-dev.20260914-8d85527a0/"
	vozWasmMJS    = vozPastaOnnx + "ort-wasm-simd-threaded.mjs"
	vozWasm       = vozPastaOnnx + "ort-wasm-simd-threaded.wasm"
	vozModelos    = "modelos/"
	vozModelo     = "whisper-small-2d67713f"
)

// arquivosDaVoz e a lista fixa: so o que esta aqui e baixado e so o que esta aqui e servido.
// Os menores vem primeiro, para um erro de endereco aparecer logo.
var arquivosDaVoz = []arquivoDeVoz{
	{vozModelos + vozModelo + "/preprocessor_config.json", origemWhisper + "preprocessor_config.json", "a6a76d28c93edb273669eb9e0b0636a2bddbb1272c3261e47b7ca6dfdbac1b8d", 339},
	{vozModelos + vozModelo + "/config.json", origemWhisper + "config.json", "5a6429d21d7a3379dd0861b74510f9f7076f32b563bffc9fcb072482d55ab3be", 2232},
	{vozModelos + vozModelo + "/generation_config.json", origemWhisper + "generation_config.json", "0b7407a4e53a677f826e03c75d409e6f830663932bf43dda3b08c5efa2223279", 3837},
	{vozWasmMJS, origemOnnx + "ort-wasm-simd-threaded.mjs", "c57ca56328877353a575e51bbca6f18450027d6c9bf2307a2cb2c41363b4de9f", 24381},
	{vozModelos + vozModelo + "/tokenizer_config.json", origemWhisper + "tokenizer_config.json", "2a4c4281cf9f51ac6ccc406fdc711a087afe6530f671fa7b80953edc498275ce", 282683},
	{vozBiblioteca, origemTransformers + "transformers.min.js", "8b5a3171f8876bab2b1820a7d65eb87628e3d30bb5ea85b9dd398e76eb34decb", 587917},
	{vozModelos + vozModelo + "/tokenizer.json", origemWhisper + "tokenizer.json", "27fc476bfe7f17299480be2273fc0608e4d5a99aba2ab5dec5374b4482d1a566", 2480466},
	{vozWasm, origemOnnx + "ort-wasm-simd-threaded.wasm", "06ba057753da3847e4c24f02d91ab133455b0817c69a44993a9a53a2146df9e3", 14264838},
	{vozModelos + vozModelo + "/onnx/encoder_model_quantized.onnx", origemWhisper + "onnx/encoder_model_quantized.onnx", "969f5ac12974340386bf7a02ea6626003e5e2dee396ffc6ab0eec282bf55ba06", 92324809},
	{vozModelos + vozModelo + "/onnx/decoder_model_merged_quantized.onnx", origemWhisper + "onnx/decoder_model_merged_quantized.onnx", "fcfc6100dc7339e7507e10f8b274350be7c4f8d8b575f0293f94cc0e156d6d24", 156780950},
}

const (
	vozTentativas = 4                // quantas vezes tenta cada arquivo antes de desistir
	vozSemSinal   = 60 * time.Second // sem chegar nenhum byte por esse tempo, a tentativa cai
)

// errVozDiferente: o arquivo chegou, mas nao e o que a lista diz. Nunca e servido.
var errVozDiferente = errors.New("o arquivo veio diferente do esperado")

// errVozGravar: o problema e no disco deste computador, nao na internet.
var errVozGravar = errors.New("não consegui gravar no disco")

// servicoDeVoz baixa, confere e serve os arquivos da voz.
type servicoDeVoz struct {
	pasta    string
	arquivos []arquivoDeVoz
	cliente  *http.Client
	esperas  []time.Duration // pausa antes de cada nova tentativa

	mu         sync.Mutex
	pronto     bool
	baixando   bool
	erro       string
	feito      int64           // bytes de arquivos ja conferidos
	atual      int64           // bytes do arquivo que esta sendo baixado agora
	conferidos map[string]bool // so estes sao servidos
}

func novoServicoDeVoz(pasta string, arquivos []arquivoDeVoz, cliente *http.Client) *servicoDeVoz {
	return &servicoDeVoz{
		pasta:      pasta,
		arquivos:   arquivos,
		cliente:    cliente,
		esperas:    []time.Duration{2 * time.Second, 6 * time.Second, 15 * time.Second},
		conferidos: map[string]bool{},
	}
}

// clienteDaVoz so segue redirecionamento para HTTPS e desiste de servidor mudo.
func clienteDaVoz() *http.Client {
	transporte := http.DefaultTransport.(*http.Transport).Clone()
	transporte.ResponseHeaderTimeout = 30 * time.Second
	return &http.Client{
		Transport: transporte,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return errors.New("redirecionamentos demais")
			}
			if req.URL.Scheme != "https" {
				return errors.New("redirecionamento para endereço sem HTTPS")
			}
			return nil
		},
	}
}

// vozDoPainel e o servico usado pelo programa. Os testes criam o deles, com outra pasta.
var vozDoPainel = novoServicoDeVoz(filepath.Join(pastaDados, "voz"), arquivosDaVoz, clienteDaVoz())

// rotasDeVoz registra as rotas do reconhecimento de voz.
func (a *App) rotasDeVoz(mux *http.ServeMux) {
	mux.HandleFunc("/api/voz/estado", vozDoPainel.rotaEstado)
	mux.HandleFunc("/api/voz/preparar", vozDoPainel.rotaPreparar)
	mux.HandleFunc("/voz/", vozDoPainel.rotaArquivo)
}

// prepararVoz roda em segundo plano ao ligar: baixa o que o reconhecimento de voz precisa.
// Com ATENDENTE_SEM_VOZ definida, nao baixa nada ao ligar (copias de teste): o download
// so comeca quando alguem clicar no microfone.
func (a *App) prepararVoz() {
	if strings.TrimSpace(os.Getenv("ATENDENTE_SEM_VOZ")) != "" {
		return
	}
	if vozDoPainel.comecar() {
		vozDoPainel.executar()
	}
}

// ---------- estado ----------

func (v *servicoDeVoz) total() int64 {
	var t int64
	for _, arq := range v.arquivos {
		t += arq.Tamanho
	}
	return t
}

// estado e o que a pagina recebe em /api/voz/estado.
func (v *servicoDeVoz) estado() map[string]any {
	total := v.total()
	v.mu.Lock()
	defer v.mu.Unlock()
	progresso := 0
	switch {
	case v.pronto:
		progresso = 100
	case total > 0:
		progresso = int((v.feito + v.atual) * 100 / total)
		if progresso > 99 {
			progresso = 99
		}
	}
	return map[string]any{
		"pronto":     v.pronto,
		"baixando":   v.baixando,
		"progresso":  progresso,
		"erro":       v.erro,
		"tamanho_mb": int((total + 500_000) / 1_000_000),
		// Onde a pagina acha cada parte. So faz sentido usar quando pronto for verdadeiro.
		"arquivos": map[string]string{
			"biblioteca": "/voz/" + vozBiblioteca,
			"wasm_mjs":   "/voz/" + vozWasmMJS,
			"wasm":       "/voz/" + vozWasm,
			"modelos":    "/voz/" + vozModelos,
			"modelo":     vozModelo,
		},
	}
}

// comecar marca o inicio de um preparo. Devolve falso se ja esta pronto ou ja tem um em curso.
func (v *servicoDeVoz) comecar() bool {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.pronto || v.baixando {
		return false
	}
	v.baixando = true
	v.erro = ""
	v.feito, v.atual = 0, 0
	return true
}

// executar faz o preparo inteiro. Nunca derruba o programa: qualquer falha vira um aviso
// no estado e uma linha no log, e o resto do painel segue normal.
func (v *servicoDeVoz) executar() {
	var err error
	defer func() {
		if p := recover(); p != nil {
			err = fmt.Errorf("falha inesperada: %v", p)
		}
		v.mu.Lock()
		v.baixando = false
		v.atual = 0
		if err != nil {
			v.erro = avisoDaVoz(err)
		} else {
			v.pronto = true
		}
		v.mu.Unlock()
		if err != nil {
			log.Printf("VOZ: o reconhecimento de voz não ficou pronto: %v", err)
		}
	}()
	err = v.prepararArquivos()
}

// avisoDaVoz transforma a falha em uma frase que a pessoa entende e sabe o que fazer.
func avisoDaVoz(err error) string {
	switch {
	case errors.Is(err, errVozDiferente):
		return "Um arquivo da voz veio diferente do esperado e foi descartado. Tente de novo mais tarde."
	case errors.Is(err, errVozGravar):
		return "Não consegui guardar os arquivos da voz neste computador. Confira se há espaço livre no disco e tente de novo."
	}
	return "Não consegui baixar o que a voz precisa. Confira a internet e tente de novo."
}

// prepararArquivos confere o que ja esta no disco e baixa o que falta.
func (v *servicoDeVoz) prepararArquivos() error {
	if err := os.MkdirAll(v.pasta, 0o700); err != nil {
		return fmt.Errorf("%w: %v", errVozGravar, err)
	}
	var faltam []arquivoDeVoz
	var bytesQueFaltam int64
	for _, arq := range v.arquivos {
		if v.confere(v.noDisco(arq), arq) {
			v.concluir(arq)
			continue
		}
		faltam = append(faltam, arq)
		bytesQueFaltam += arq.Tamanho
	}
	if len(faltam) == 0 {
		log.Printf("VOZ: reconhecimento de voz pronto (arquivos já baixados e conferidos).")
		return nil
	}
	inicio := time.Now()
	log.Printf("VOZ: baixando o reconhecimento de voz (%d arquivos, %d MB). Só acontece uma vez.", len(faltam), (bytesQueFaltam+500_000)/1_000_000)
	for _, arq := range faltam {
		if err := v.baixar(arq); err != nil {
			return fmt.Errorf("%s: %w", arq.Caminho, err)
		}
		v.concluir(arq)
	}
	log.Printf("VOZ: reconhecimento de voz pronto em %s.", time.Since(inicio).Round(time.Second))
	return nil
}

func (v *servicoDeVoz) noDisco(arq arquivoDeVoz) string {
	return filepath.Join(v.pasta, filepath.FromSlash(arq.Caminho))
}

func (v *servicoDeVoz) concluir(arq arquivoDeVoz) {
	v.mu.Lock()
	v.conferidos[arq.Caminho] = true
	v.feito += arq.Tamanho
	v.atual = 0
	v.mu.Unlock()
}

func (v *servicoDeVoz) andar(bytesDoAtual int64) {
	v.mu.Lock()
	v.atual = bytesDoAtual
	v.mu.Unlock()
}

// confere diz se o arquivo no disco tem o tamanho e o SHA-256 da lista.
func (v *servicoDeVoz) confere(caminho string, arq arquivoDeVoz) bool {
	st, err := os.Stat(caminho)
	if err != nil || st.IsDir() || st.Size() != arq.Tamanho {
		return false
	}
	soma, err := sha256DoArquivo(caminho)
	return err == nil && strings.EqualFold(soma, arq.SHA256)
}

func sha256DoArquivo(caminho string) (string, error) {
	f, err := os.Open(caminho)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// ---------- download ----------

// baixar traz um arquivo para "nome.parte", confere e so entao troca o nome. Se a
// internet cair no meio, a proxima tentativa continua de onde parou.
func (v *servicoDeVoz) baixar(arq arquivoDeVoz) error {
	if !strings.HasPrefix(arq.Origem, "https://") {
		return fmt.Errorf("endereço sem HTTPS: %s", arq.Origem)
	}
	destino := v.noDisco(arq)
	if err := os.MkdirAll(filepath.Dir(destino), 0o700); err != nil {
		return fmt.Errorf("%w: %v", errVozGravar, err)
	}
	parte := destino + ".parte"
	var ultimo error
	diferentes := 0
	for tentativa := 0; tentativa < vozTentativas; tentativa++ {
		if tentativa > 0 {
			time.Sleep(v.esperas[min(tentativa-1, len(v.esperas)-1)])
		}
		ultimo = v.baixarUmaVez(arq, parte)
		if ultimo == nil {
			if err := os.Rename(parte, destino); err != nil {
				return fmt.Errorf("%w: %v", errVozGravar, err)
			}
			return nil
		}
		log.Printf("VOZ: tentativa %d de %s falhou: %v", tentativa+1, arq.Caminho, ultimo)
		if errors.Is(ultimo, errVozGravar) {
			break
		}
		if errors.Is(ultimo, errVozDiferente) {
			if diferentes++; diferentes >= 2 {
				break
			}
		}
	}
	return ultimo
}

func (v *servicoDeVoz) baixarUmaVez(arq arquivoDeVoz, parte string) error {
	// O que ja chegou em uma tentativa anterior e aproveitado.
	var ja int64
	if st, err := os.Stat(parte); err == nil && !st.IsDir() && st.Size() > 0 && st.Size() < arq.Tamanho {
		ja = st.Size()
	} else {
		_ = os.Remove(parte)
	}

	ctx, cancelar := context.WithCancel(context.Background())
	defer cancelar()
	vigia := time.AfterFunc(vozSemSinal, cancelar)
	defer vigia.Stop()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, arq.Origem, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept-Encoding", "identity") // os tamanhos da lista sao do arquivo como ele e
	if ja > 0 {
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-", ja))
	}
	resp, err := v.cliente.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	switch {
	case resp.StatusCode == http.StatusOK:
		ja = 0 // o servidor mandou o arquivo inteiro: comeca do zero
	case resp.StatusCode == http.StatusPartialContent && ja > 0 &&
		strings.HasPrefix(resp.Header.Get("Content-Range"), fmt.Sprintf("bytes %d-", ja)):
		// continua de onde parou
	default:
		_ = os.Remove(parte)
		return fmt.Errorf("o servidor respondeu %d", resp.StatusCode)
	}
	if resp.ContentLength >= 0 && ja+resp.ContentLength != arq.Tamanho {
		_ = os.Remove(parte)
		return fmt.Errorf("%w: tamanho %d, esperava %d", errVozDiferente, ja+resp.ContentLength, arq.Tamanho)
	}

	modo := os.O_CREATE | os.O_WRONLY | os.O_TRUNC
	if ja > 0 {
		modo = os.O_WRONLY | os.O_APPEND
	}
	f, err := os.OpenFile(parte, modo, 0o600)
	if err != nil {
		return fmt.Errorf("%w: %v", errVozGravar, err)
	}
	fechado := false
	defer func() {
		if !fechado {
			f.Close()
		}
	}()

	v.andar(ja)
	buf := make([]byte, 256<<10)
	for {
		n, errLer := resp.Body.Read(buf)
		if n > 0 {
			vigia.Reset(vozSemSinal)
			if ja+int64(n) > arq.Tamanho {
				f.Close()
				fechado = true
				_ = os.Remove(parte)
				return fmt.Errorf("%w: maior que %d bytes", errVozDiferente, arq.Tamanho)
			}
			if _, err := f.Write(buf[:n]); err != nil {
				return fmt.Errorf("%w: %v", errVozGravar, err)
			}
			ja += int64(n)
			v.andar(ja)
		}
		if errLer == io.EOF {
			break
		}
		if errLer != nil {
			return errLer // o que ja chegou fica em "nome.parte" para a proxima tentativa
		}
	}
	if err := f.Sync(); err != nil {
		return fmt.Errorf("%w: %v", errVozGravar, err)
	}
	fechado = true
	if err := f.Close(); err != nil {
		return fmt.Errorf("%w: %v", errVozGravar, err)
	}
	if ja != arq.Tamanho {
		return fmt.Errorf("chegaram %d de %d bytes", ja, arq.Tamanho)
	}
	soma, err := sha256DoArquivo(parte)
	if err != nil {
		return fmt.Errorf("%w: %v", errVozGravar, err)
	}
	if !strings.EqualFold(soma, arq.SHA256) {
		_ = os.Remove(parte)
		return fmt.Errorf("%w: SHA-256 %s", errVozDiferente, soma)
	}
	return nil
}

// ---------- rotas ----------

// rotaEstado: GET /api/voz/estado.
func (v *servicoDeVoz) rotaEstado(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		responderErro(w, http.StatusMethodNotAllowed, "Use os botões da página.")
		return
	}
	responderOK(w, v.estado())
}

// rotaPreparar: POST /api/voz/preparar. Comeca o download, ou tenta de novo depois de uma falha.
func (v *servicoDeVoz) rotaPreparar(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		responderErro(w, http.StatusMethodNotAllowed, "Use os botões da página.")
		return
	}
	if v.comecar() {
		go v.executar()
	}
	responderOK(w, v.estado())
}

// tipoDaVoz e o tipo fixo de cada arquivo, pela extensao. Nada e adivinhado pelo conteudo.
func tipoDaVoz(caminho string) string {
	switch strings.ToLower(path.Ext(caminho)) {
	case ".wasm":
		return "application/wasm"
	case ".js", ".mjs":
		return "text/javascript; charset=utf-8"
	case ".json":
		return "application/json"
	}
	return "application/octet-stream"
}

// rotaArquivo: GET /voz/... Entrega so os arquivos da lista fixa, e so depois de
// conferidos. O endereco pedido nunca vira caminho de disco: ele so e procurado na lista.
func (v *servicoDeVoz) rotaArquivo(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "método não aceito", http.StatusMethodNotAllowed)
		return
	}
	// Rota nova nasce fechada: a tela de entrada nao precisa da voz.
	if usuarioDe(r) == nil {
		http.Error(w, "Entre no painel para continuar.", http.StatusUnauthorized)
		return
	}
	pedido := strings.TrimPrefix(r.URL.Path, "/voz/")
	var arq *arquivoDeVoz
	for i := range v.arquivos {
		if v.arquivos[i].Caminho == pedido {
			arq = &v.arquivos[i]
			break
		}
	}
	v.mu.Lock()
	liberado := arq != nil && v.conferidos[arq.Caminho]
	v.mu.Unlock()
	if !liberado {
		http.NotFound(w, r)
		return
	}
	f, err := os.Open(v.noDisco(*arq))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil || st.IsDir() {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", tipoDaVoz(arq.Caminho))
	// O caminho leva a versao no nome e o conteudo nunca muda: o navegador pode guardar.
	w.Header().Set("Cache-Control", "private, max-age=31536000, immutable")
	w.Header().Set("ETag", `"`+arq.SHA256+`"`)
	http.ServeContent(w, r, "", st.ModTime(), f)
}
