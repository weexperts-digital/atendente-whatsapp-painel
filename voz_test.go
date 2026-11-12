package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// origemDeMentira faz o papel dos servidores de onde a voz e baixada. Nada sai para a internet.
type origemDeMentira struct {
	mu       sync.Mutex
	conteudo map[string][]byte // o que cada caminho devolve
	pedidos  []string          // "GET /caminho [Range]"
	cortar   map[string]int    // corta a conexao depois de N bytes, uma vez so
	segurar  chan struct{}     // se nao for nil, segura o arquivo maior na metade ate fechar
	srv      *httptest.Server
}

func novaOrigemDeMentira(t *testing.T) *origemDeMentira {
	t.Helper()
	o := &origemDeMentira{conteudo: map[string][]byte{}, cortar: map[string]int{}}
	o.srv = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		o.mu.Lock()
		dados, existe := o.conteudo[r.URL.Path]
		o.pedidos = append(o.pedidos, strings.TrimSpace(r.Method+" "+r.URL.Path+" "+r.Header.Get("Range")))
		corte, temCorte := o.cortar[r.URL.Path]
		delete(o.cortar, r.URL.Path)
		segurar := o.segurar
		o.mu.Unlock()
		if !existe {
			http.NotFound(w, r)
			return
		}
		inicio := 0
		if faixa := r.Header.Get("Range"); faixa != "" {
			if _, err := fmt.Sscanf(faixa, "bytes=%d-", &inicio); err != nil || inicio >= len(dados) {
				w.WriteHeader(http.StatusRequestedRangeNotSatisfiable)
				return
			}
			w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", inicio, len(dados)-1, len(dados)))
			w.Header().Set("Content-Length", fmt.Sprint(len(dados)-inicio))
			w.WriteHeader(http.StatusPartialContent)
			_, _ = w.Write(dados[inicio:])
			return
		}
		w.Header().Set("Content-Length", fmt.Sprint(len(dados)))
		if temCorte {
			_, _ = w.Write(dados[:corte])
			w.(http.Flusher).Flush()
			panic(http.ErrAbortHandler) // derruba a conexao no meio do arquivo
		}
		if segurar != nil && strings.HasSuffix(r.URL.Path, ".onnx") {
			_, _ = w.Write(dados[:len(dados)/2])
			w.(http.Flusher).Flush()
			<-segurar
			_, _ = w.Write(dados[len(dados)/2:])
			return
		}
		_, _ = w.Write(dados)
	}))
	t.Cleanup(o.srv.Close)
	return o
}

func (o *origemDeMentira) contar(trecho string) int {
	o.mu.Lock()
	defer o.mu.Unlock()
	n := 0
	for _, p := range o.pedidos {
		if strings.Contains(p, trecho) {
			n++
		}
	}
	return n
}

func somaDe(dados []byte) string {
	s := sha256.Sum256(dados)
	return hex.EncodeToString(s[:])
}

// vozDeTeste monta um servico de voz com arquivos pequenos, servidos pela origem de mentira.
func vozDeTeste(t *testing.T) (*servicoDeVoz, *origemDeMentira, map[string][]byte) {
	t.Helper()
	o := novaOrigemDeMentira(t)
	conteudo := map[string][]byte{
		"lib/biblioteca.min.js":         []byte("export const versao = 'de mentira';\n"),
		"lib/motor.mjs":                 []byte("export default function () {}\n"),
		"lib/motor.wasm":                append([]byte{0, 'a', 's', 'm', 1, 0, 0, 0}, bytes.Repeat([]byte{7}, 5000)...),
		"modelos/m/config.json":         []byte(`{"model_type":"whisper"}`),
		"modelos/m/onnx/encoder_q.onnx": bytes.Repeat([]byte("onnx de mentira "), 4000),
	}
	var lista []arquivoDeVoz
	for _, nome := range []string{"modelos/m/config.json", "lib/motor.mjs", "lib/biblioteca.min.js", "lib/motor.wasm", "modelos/m/onnx/encoder_q.onnx"} {
		dados := conteudo[nome]
		o.conteudo["/origem/"+nome] = dados
		lista = append(lista, arquivoDeVoz{Caminho: nome, Origem: o.srv.URL + "/origem/" + nome, SHA256: somaDe(dados), Tamanho: int64(len(dados))})
	}
	v := novoServicoDeVoz(filepath.Join(t.TempDir(), "voz"), lista, o.srv.Client())
	v.esperas = []time.Duration{time.Millisecond}
	return v, o, conteudo
}

func prepararDeVerdade(t *testing.T, v *servicoDeVoz) map[string]any {
	t.Helper()
	if !v.comecar() {
		t.Fatalf("o preparo não começou")
	}
	v.executar()
	return v.estado()
}

// pedirArquivo chama a rota dos arquivos como o painel chama, com ou sem alguem logado.
func pedirArquivo(v *servicoDeVoz, metodo, caminho string, logado bool) *httptest.ResponseRecorder {
	r := httptest.NewRequest(metodo, "http://localhost:8787/voz/x", nil)
	r.URL.Path = caminho // sem passar pela limpeza de caminho, para testar o pior caso
	if logado {
		r = r.WithContext(context.WithValue(r.Context(), chaveUsuario{}, &Usuario{ID: "u1", Papel: papelAtendente}))
	}
	w := httptest.NewRecorder()
	v.rotaArquivo(w, r)
	return w
}

func TestVozBaixaConfereEGuarda(t *testing.T) {
	v, o, conteudo := vozDeTeste(t)

	antes := v.estado()
	if antes["pronto"] != false || antes["baixando"] != false || antes["progresso"] != 0 || antes["erro"] != "" {
		t.Fatalf("antes de baixar, o estado devia estar parado e sem erro: %v", antes)
	}

	depois := prepararDeVerdade(t, v)
	if depois["pronto"] != true || depois["baixando"] != false || depois["progresso"] != 100 || depois["erro"] != "" {
		t.Fatalf("depois de baixar, devia estar pronto: %v", depois)
	}
	for nome, dados := range conteudo {
		noDisco, err := os.ReadFile(filepath.Join(v.pasta, filepath.FromSlash(nome)))
		if err != nil || !bytes.Equal(noDisco, dados) {
			t.Errorf("%s não ficou igual no disco (erro: %v)", nome, err)
		}
		if existeArquivo(filepath.Join(v.pasta, filepath.FromSlash(nome)) + ".parte") {
			t.Errorf("%s deixou um arquivo temporário para trás", nome)
		}
	}
	if n := o.contar("GET "); n != len(conteudo) {
		t.Errorf("esperava um pedido por arquivo, foram %d", n)
	}

	// Ligando de novo, o que ja esta conferido no disco nao e baixado outra vez.
	outro := novoServicoDeVoz(v.pasta, v.arquivos, o.srv.Client())
	if e := prepararDeVerdade(t, outro); e["pronto"] != true {
		t.Fatalf("segunda vez devia ficar pronto só conferindo o disco: %v", e)
	}
	if n := o.contar("GET "); n != len(conteudo) {
		t.Errorf("a segunda vez baixou de novo: %d pedidos no total", n)
	}

	// Arquivo mexido no disco (mesmo tamanho, outro conteudo) e baixado de novo.
	alvo := filepath.Join(v.pasta, "lib", "motor.wasm")
	if err := os.WriteFile(alvo, bytes.Repeat([]byte{9}, len(conteudo["lib/motor.wasm"])), 0o600); err != nil {
		t.Fatal(err)
	}
	terceiro := novoServicoDeVoz(v.pasta, v.arquivos, o.srv.Client())
	if e := prepararDeVerdade(t, terceiro); e["pronto"] != true {
		t.Fatalf("devia consertar o arquivo mexido: %v", e)
	}
	if noDisco, _ := os.ReadFile(alvo); !bytes.Equal(noDisco, conteudo["lib/motor.wasm"]) {
		t.Errorf("o arquivo mexido não foi trocado pelo certo")
	}
	if n := o.contar("GET /origem/lib/motor.wasm"); n != 2 {
		t.Errorf("só o arquivo mexido devia ser baixado de novo, foram %d pedidos dele", n)
	}
}

func TestVozRecusaArquivoDiferente(t *testing.T) {
	for nome, falso := range map[string]func(certo []byte) []byte{
		"mesmo tamanho, outro conteúdo": func(certo []byte) []byte { return bytes.Repeat([]byte{'x'}, len(certo)) },
		"maior que o esperado":          func(certo []byte) []byte { return append(append([]byte{}, certo...), "sobra"...) },
		"menor que o esperado":          func(certo []byte) []byte { return certo[:len(certo)/2] },
	} {
		t.Run(nome, func(t *testing.T) {
			v, o, conteudo := vozDeTeste(t)
			o.conteudo["/origem/lib/motor.wasm"] = falso(conteudo["lib/motor.wasm"])

			e := prepararDeVerdade(t, v)
			if e["pronto"] != false || e["baixando"] != false {
				t.Fatalf("com um arquivo trocado, a voz não pode ficar pronta: %v", e)
			}
			if aviso, _ := e["erro"].(string); !strings.Contains(aviso, "diferente do esperado") {
				t.Errorf("o aviso devia dizer que o arquivo veio diferente: %q", aviso)
			}
			destino := filepath.Join(v.pasta, "lib", "motor.wasm")
			if existeArquivo(destino) || existeArquivo(destino+".parte") {
				t.Errorf("o arquivo recusado ficou no disco")
			}
			if w := pedirArquivo(v, "GET", "/voz/lib/motor.wasm", true); w.Code != http.StatusNotFound {
				t.Errorf("o arquivo recusado foi servido: código %d", w.Code)
			}

			// Com a origem consertada, tentar de novo resolve.
			o.conteudo["/origem/lib/motor.wasm"] = conteudo["lib/motor.wasm"]
			if e := prepararDeVerdade(t, v); e["pronto"] != true || e["erro"] != "" {
				t.Errorf("tentar de novo devia resolver: %v", e)
			}
		})
	}
}

func TestVozContinuaDeOndeParou(t *testing.T) {
	v, o, conteudo := vozDeTeste(t)
	const nome = "modelos/m/onnx/encoder_q.onnx"
	o.cortar["/origem/"+nome] = 20000 // a conexao cai depois de 20000 bytes

	if e := prepararDeVerdade(t, v); e["pronto"] != true {
		t.Fatalf("devia terminar mesmo com a queda no meio: %v", e)
	}
	noDisco, _ := os.ReadFile(filepath.Join(v.pasta, filepath.FromSlash(nome)))
	if !bytes.Equal(noDisco, conteudo[nome]) {
		t.Fatalf("o arquivo retomado não ficou igual ao original")
	}
	if o.contar("GET /origem/"+nome+" bytes=") != 1 {
		t.Errorf("a segunda tentativa devia pedir só o que faltava: %v", o.pedidos)
	}
}

func TestVozSemInternet(t *testing.T) {
	v, o, _ := vozDeTeste(t)
	o.srv.Close() // ninguem atende: e o mesmo que estar sem internet

	e := prepararDeVerdade(t, v)
	if e["pronto"] != false || e["baixando"] != false {
		t.Fatalf("sem internet, a voz só fica indisponível: %v", e)
	}
	if aviso, _ := e["erro"].(string); !strings.Contains(aviso, "Confira a internet") {
		t.Errorf("o aviso devia mandar conferir a internet: %q", aviso)
	}
	if w := pedirArquivo(v, "GET", "/voz/lib/motor.wasm", true); w.Code != http.StatusNotFound {
		t.Errorf("sem ter baixado, não há o que servir: código %d", w.Code)
	}
}

func TestVozSoBaixaPorHTTPS(t *testing.T) {
	semTLS := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("oi")) }))
	defer semTLS.Close()
	v := novoServicoDeVoz(filepath.Join(t.TempDir(), "voz"), []arquivoDeVoz{{Caminho: "a.js", Origem: semTLS.URL + "/a.js", SHA256: somaDe([]byte("oi")), Tamanho: 2}}, semTLS.Client())
	v.esperas = []time.Duration{time.Millisecond}
	if e := prepararDeVerdade(t, v); e["pronto"] != false {
		t.Errorf("endereço sem HTTPS não pode ser aceito: %v", e)
	}
}

func TestVozMostraOProgresso(t *testing.T) {
	v, o, _ := vozDeTeste(t)
	o.segurar = make(chan struct{})

	w := httptest.NewRecorder()
	v.rotaPreparar(w, httptest.NewRequest("POST", "/api/voz/preparar", strings.NewReader("{}")))
	var resp map[string]any
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil || resp["ok"] != true || resp["baixando"] != true {
		t.Fatalf("preparar devia começar o download: %v %v", resp, err)
	}
	// Pedir de novo enquanto baixa nao comeca outro download.
	v.rotaPreparar(httptest.NewRecorder(), httptest.NewRequest("POST", "/api/voz/preparar", strings.NewReader("{}")))

	meio := false
	for i := 0; i < 400 && !meio; i++ {
		e := v.estado()
		p, _ := e["progresso"].(int)
		meio = e["baixando"] == true && e["pronto"] == false && p > 0 && p < 100
		time.Sleep(5 * time.Millisecond)
	}
	if !meio {
		t.Errorf("no meio do download o estado devia mostrar baixando com progresso parcial: %v", v.estado())
	}
	close(o.segurar)
	for i := 0; i < 400 && v.estado()["pronto"] != true; i++ {
		time.Sleep(5 * time.Millisecond)
	}
	if e := v.estado(); e["pronto"] != true || e["baixando"] != false || e["progresso"] != 100 {
		t.Fatalf("devia terminar pronto: %v", e)
	}
	if n := o.contar("GET /origem/lib/motor.wasm"); n != 1 {
		t.Errorf("cada arquivo devia ser pedido uma vez só, motor.wasm foi %d", n)
	}

	w = httptest.NewRecorder()
	v.rotaEstado(w, httptest.NewRequest("GET", "/api/voz/estado", nil))
	resp = nil
	_ = json.NewDecoder(w.Body).Decode(&resp)
	arquivos, _ := resp["arquivos"].(map[string]any)
	if resp["ok"] != true || resp["pronto"] != true || resp["tamanho_mb"] == nil || arquivos["biblioteca"] == nil || arquivos["modelo"] == nil {
		t.Errorf("estado veio incompleto: %v", resp)
	}
	w = httptest.NewRecorder()
	v.rotaPreparar(w, httptest.NewRequest("GET", "/api/voz/preparar", nil))
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("preparar só aceita POST: código %d", w.Code)
	}
}

func TestVozServeSoOQueEstaNaLista(t *testing.T) {
	v, _, conteudo := vozDeTeste(t)
	if e := prepararDeVerdade(t, v); e["pronto"] != true {
		t.Fatalf("preparo: %v", e)
	}

	tipos := map[string]string{
		"lib/biblioteca.min.js":         "text/javascript; charset=utf-8",
		"lib/motor.mjs":                 "text/javascript; charset=utf-8",
		"lib/motor.wasm":                "application/wasm",
		"modelos/m/config.json":         "application/json",
		"modelos/m/onnx/encoder_q.onnx": "application/octet-stream",
	}
	for nome, tipo := range tipos {
		w := pedirArquivo(v, "GET", "/voz/"+nome, true)
		if w.Code != http.StatusOK || !bytes.Equal(w.Body.Bytes(), conteudo[nome]) {
			t.Errorf("%s: código %d ou conteúdo errado", nome, w.Code)
		}
		if got := w.Header().Get("Content-Type"); got != tipo {
			t.Errorf("%s: tipo %q, esperava %q", nome, got, tipo)
		}
		if cache := w.Header().Get("Cache-Control"); !strings.Contains(cache, "max-age=31536000") || !strings.Contains(cache, "immutable") {
			t.Errorf("%s: sem cache longo: %q", nome, cache)
		}
		if w := pedirArquivo(v, "GET", "/voz/"+nome, false); w.Code != http.StatusUnauthorized {
			t.Errorf("%s sem login: código %d, esperava 401", nome, w.Code)
		}
	}

	// Um pedido de pedaco (o navegador faz isso com arquivos grandes) funciona.
	r := httptest.NewRequest("GET", "/voz/lib/motor.wasm", nil)
	r.Header.Set("Range", "bytes=0-3")
	r = r.WithContext(context.WithValue(r.Context(), chaveUsuario{}, &Usuario{ID: "u1"}))
	w := httptest.NewRecorder()
	v.rotaArquivo(w, r)
	if w.Code != http.StatusPartialContent || w.Body.String() != "\x00asm" {
		t.Errorf("pedido de pedaço: código %d, corpo %q", w.Code, w.Body.String())
	}

	// Coisas que estao no disco, dentro e fora da pasta da voz, mas nao estao na lista.
	fora := filepath.Join(filepath.Dir(v.pasta), "atendente.db")
	for _, p := range []string{fora, filepath.Join(v.pasta, "segredo.txt"), filepath.Join(v.pasta, "lib", "motor.wasm.parte")} {
		if err := os.WriteFile(p, []byte("nao pode sair"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	for _, caminho := range []string{
		"/voz/", "/voz/segredo.txt", "/voz/lib/", "/voz/lib", "/voz/lib/motor.wasm.parte", "/voz/nao-existe.js",
		"/voz/../atendente.db", "/voz/lib/../../atendente.db", "/voz/..%2fatendente.db", "/voz/%2e%2e/atendente.db",
		`/voz/..\atendente.db`, "/voz//lib/motor.wasm", "/voz/lib/./motor.wasm", "/voz/LIB/MOTOR.WASM",
		"/voz/" + fora, "/voz/lib/motor.wasm/", "/voz/lib/motor.wasm%00.js",
	} {
		if w := pedirArquivo(v, "GET", caminho, true); w.Code != http.StatusNotFound || strings.Contains(w.Body.String(), "nao pode sair") {
			t.Errorf("%q devia dar 404 sem entregar nada: código %d", caminho, w.Code)
		}
	}
	for _, metodo := range []string{"POST", "PUT", "DELETE"} {
		if w := pedirArquivo(v, metodo, "/voz/lib/motor.wasm", true); w.Code != http.StatusMethodNotAllowed {
			t.Errorf("%s nos arquivos da voz: código %d, esperava 405", metodo, w.Code)
		}
	}
}

func TestTipoDaVoz(t *testing.T) {
	for caminho, tipo := range map[string]string{
		"a/ort-wasm-simd-threaded.wasm": "application/wasm",
		"a/ort-wasm-simd-threaded.mjs":  "text/javascript; charset=utf-8",
		"a/transformers.min.js":         "text/javascript; charset=utf-8",
		"m/config.json":                 "application/json",
		"m/onnx/encoder.onnx":           "application/octet-stream",
		"m/sem-extensao":                "application/octet-stream",
		"m/pagina.html":                 "application/octet-stream", // nunca vira pagina
	} {
		if got := tipoDaVoz(caminho); got != tipo {
			t.Errorf("%s: %q, esperava %q", caminho, got, tipo)
		}
	}
}

// A lista de verdade: enderecos fixos com HTTPS e versao exata, SHA-256 completo e caminhos limpos.
func TestListaDaVozEFixa(t *testing.T) {
	vistos := map[string]bool{}
	var total int64
	for _, arq := range arquivosDaVoz {
		if !strings.HasPrefix(arq.Origem, "https://cdn.jsdelivr.net/npm/") && !strings.HasPrefix(arq.Origem, "https://huggingface.co/") {
			t.Errorf("%s: origem fora do combinado: %s", arq.Caminho, arq.Origem)
		}
		if strings.Contains(arq.Origem, "@latest") || strings.Contains(arq.Origem, "/resolve/main/") {
			t.Errorf("%s: a origem tem de apontar para uma versão exata: %s", arq.Caminho, arq.Origem)
		}
		if soma, err := hex.DecodeString(arq.SHA256); err != nil || len(soma) != sha256.Size {
			t.Errorf("%s: SHA-256 inválido", arq.Caminho)
		}
		if arq.Tamanho <= 0 {
			t.Errorf("%s: sem tamanho", arq.Caminho)
		}
		if arq.Caminho == "" || strings.Contains(arq.Caminho, "..") || strings.HasPrefix(arq.Caminho, "/") || strings.Contains(arq.Caminho, `\`) || vistos[arq.Caminho] {
			t.Errorf("caminho ruim ou repetido: %q", arq.Caminho)
		}
		vistos[arq.Caminho] = true
		total += arq.Tamanho
	}
	for _, precisa := range []string{vozBiblioteca, vozWasmMJS, vozWasm, vozModelos + vozModelo + "/config.json"} {
		if !vistos[precisa] {
			t.Errorf("falta na lista o que a página usa: %s", precisa)
		}
	}
	if total < 200_000_000 || total > 320_000_000 {
		t.Errorf("tamanho total inesperado: %d bytes", total)
	}
	if mb := vozDoPainel.estado()["tamanho_mb"]; mb != int((total+500_000)/1_000_000) {
		t.Errorf("tamanho_mb não bate com a lista: %v", mb)
	}
}

// Pelo painel de verdade, com o filtro de acesso na frente: a voz so existe para quem entrou.
func TestVozExigeLogin(t *testing.T) {
	pastaDeTeste(t)
	a := novoApp()
	a.banco = bancoDeTeste(t)
	a.cookie = "atendente_teste"
	srv := httptest.NewServer(a.protegido(a.rotas()))
	defer srv.Close()

	pedir := func(c *http.Client, metodo, caminho string) (*http.Response, map[string]any) {
		t.Helper()
		req, _ := http.NewRequest(metodo, srv.URL+caminho, strings.NewReader("{}"))
		req.Header.Set("Content-Type", "application/json")
		resp, err := c.Do(req)
		if err != nil {
			t.Fatalf("%s %s: %v", metodo, caminho, err)
		}
		defer resp.Body.Close()
		var saida map[string]any
		_ = json.NewDecoder(resp.Body).Decode(&saida)
		return resp, saida
	}
	anonimo := &http.Client{}
	for _, caminho := range []string{"/api/voz/estado", "/voz/" + vozBiblioteca, "/voz/" + vozWasm} {
		if resp, _ := pedir(anonimo, "GET", caminho); resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("sem login, %s: código %d, esperava 401", caminho, resp.StatusCode)
		}
	}
	if resp, _ := pedir(anonimo, "POST", "/api/voz/preparar"); resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("sem login, preparar: código %d, esperava 401", resp.StatusCode)
	}

	jar, _ := cookiejar.New(nil)
	dono := &http.Client{Jar: jar}
	const senha = "cavalo-bateria-grampo"
	req, _ := http.NewRequest("POST", srv.URL+"/api/primeiro-acesso", strings.NewReader(`{"nome":"Dona","email":"dona@loja.com","senha":"`+senha+`","senha2":"`+senha+`"}`))
	req.Header.Set("Content-Type", "application/json")
	if resp, err := dono.Do(req); err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("criando a conta: %v", err)
	}

	resp, e := pedir(dono, "GET", "/api/voz/estado")
	if resp.StatusCode != http.StatusOK || e["ok"] != true || e["pronto"] != false || e["baixando"] != false || e["erro"] != "" {
		t.Errorf("logado, o estado devia vir parado e sem erro (nada foi baixado no teste): %d %v", resp.StatusCode, e)
	}
	for _, campo := range []string{"pronto", "baixando", "progresso", "erro", "tamanho_mb", "arquivos"} {
		if _, tem := e[campo]; !tem {
			t.Errorf("faltou %q no estado: %v", campo, e)
		}
	}
	// Logado, mas ainda sem download: o arquivo nao existe, e o painel nao tenta adivinhar.
	if resp, _ := pedir(dono, "GET", "/voz/"+vozBiblioteca); resp.StatusCode != http.StatusNotFound {
		t.Errorf("antes do download, o arquivo da voz devia dar 404: %d", resp.StatusCode)
	}
	// Os cabecalhos que a pagina precisa para usar varios nucleos continuam vindo.
	if resp, _ := pedir(dono, "GET", "/voz/"+vozBiblioteca); resp.Header.Get("Cross-Origin-Embedder-Policy") != "require-corp" || resp.Header.Get("X-Content-Type-Options") != "nosniff" {
		t.Errorf("faltaram cabeçalhos de segurança nos arquivos da voz: %v", resp.Header)
	}
}
