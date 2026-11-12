package main

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

const chaveDeTeste = "sk-teste-0123456789abcdefghij-FIM9"

const saidaDeTeste = `{"resposta":"Olá! Posso ajudar?","transbordo":false,"motivo":"","lead":{"nome":"","necessidade":"","orcamento":"","decisor":"","prazo":"","pontuacao":10,"classificacao":"frio","etapa":"início","resumo":"","etiquetas":["primeiro contato"]},"agendamento":{"acao":"nenhuma","servico":"","data":"","hora":""}}`

// openaiDeMentira imita as duas rotas da OpenAI que o programa usa. Guarda os pedidos
// que recebeu para o teste conferir o que saiu do programa.
type openaiDeMentira struct {
	mu       sync.Mutex
	pedidos  []map[string]any
	recusar  string // "chave", "credito", "modelo", "formato" (recusa o esquema uma vez), "papel", "filtro" ou "listar"
	recusou  bool
	servidor *httptest.Server
}

func novaOpenAIDeMentira(t *testing.T) *openaiDeMentira {
	t.Helper()
	o := &openaiDeMentira{}
	erro := func(w http.ResponseWriter, status int, codigo, msg string) {
		w.WriteHeader(status)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{"message": msg, "type": "invalid_request_error", "code": codigo}})
	}
	o.servidor = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		o.mu.Lock()
		defer o.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		if r.Header.Get("Authorization") != "Bearer "+chaveDeTeste || o.recusar == "chave" {
			erro(w, http.StatusUnauthorized, "invalid_api_key", "Incorrect API key provided: sk-teste*****FIM9.")
			return
		}
		switch r.URL.Path {
		case "/models":
			if o.recusar == "listar" {
				erro(w, http.StatusForbidden, "", "You have insufficient permissions for this operation. Missing scopes: api.model.read.")
				return
			}
			_, _ = io.WriteString(w, `{"data":[{"id":"gpt-4o-mini"},{"id":"gpt-5-mini"},{"id":"gpt-5"},{"id":"gpt-4o-audio-preview"},{"id":"text-embedding-3-small"},{"id":"dall-e-3"}]}`)
		case "/chat/completions":
			var p map[string]any
			_ = json.NewDecoder(r.Body).Decode(&p)
			o.pedidos = append(o.pedidos, p)
			formato, _ := p["response_format"].(map[string]any)
			switch {
			case o.recusar == "credito":
				erro(w, http.StatusTooManyRequests, "insufficient_quota", "You exceeded your current quota, please check your plan and billing details.")
			case o.recusar == "modelo":
				erro(w, http.StatusNotFound, "model_not_found", "The model `gpt-inexistente` does not exist or you do not have access to it.")
			case o.recusar == "formato" && !o.recusou && formato["type"] == "json_schema":
				o.recusou = true
				erro(w, http.StatusBadRequest, "invalid_request_error", "Invalid schema for response_format 'saida'.")
			case o.recusar == "papel" && p["messages"].([]any)[0].(map[string]any)["role"] == "system":
				erro(w, http.StatusBadRequest, "unsupported_value", "Unsupported value: 'messages[0].role' does not support 'system' with this model.")
			case o.recusar == "filtro":
				_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{
					"finish_reason": "content_filter", "message": map[string]any{"role": "assistant", "content": nil},
				}}})
			default:
				_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{
					"finish_reason": "stop", "message": map[string]any{"role": "assistant", "content": saidaDeTeste},
				}}})
			}
		default:
			erro(w, http.StatusNotFound, "not_found", "rota desconhecida")
		}
	}))
	antes := enderecoOpenAI
	enderecoOpenAI = o.servidor.URL
	t.Cleanup(func() { enderecoOpenAI = antes; o.servidor.Close() })
	return o
}

func TestChatGPTRespondeSemFerramentas(t *testing.T) {
	o := novaOpenAIDeMentira(t)
	c := &Cerebro{}
	c.definirOpenAI(chaveDeTeste, "gpt-5-mini")

	s, modo, err := c.Responder("claude-haiku-5-5", "instruções do negócio", "Cliente: oi\n", "oi")
	if err != nil || modo != "openai" || s.Resposta != "Olá! Posso ajudar?" || len(s.Lead.Etiquetas) != 1 {
		t.Fatalf("resposta pelo ChatGPT: %+v, modo %q, erro %v", s, modo, err)
	}
	if m, _ := c.Modo(); m != "openai" {
		t.Errorf("o modo devia ser openai, veio %q", m)
	}
	p := o.pedidos[0]
	if p["model"] != "gpt-5-mini" {
		t.Errorf("com o ChatGPT vale o modelo da OpenAI, não o do Claude: %v", p["model"])
	}
	for _, proibido := range []string{"tools", "functions", "tool_choice", "web_search_options"} {
		if _, tem := p[proibido]; tem {
			t.Errorf("o pedido não pode declarar ferramentas (%s)", proibido)
		}
	}
	formato := p["response_format"].(map[string]any)
	esquema := formato["json_schema"].(map[string]any)
	if formato["type"] != "json_schema" || esquema["strict"] != true {
		t.Errorf("o pedido devia exigir o formato do esquema: %v", formato)
	}
	msgs := p["messages"].([]any)
	if len(msgs) != 2 || msgs[0].(map[string]any)["role"] != "system" || msgs[1].(map[string]any)["content"] != "Cliente: oi\n" {
		t.Errorf("mensagens do pedido: %v", msgs)
	}
}

func TestChatGPTSemEsquemaTentaJSONSimples(t *testing.T) {
	o := novaOpenAIDeMentira(t)
	o.recusar = "formato"
	bruto, err := chamarOpenAI(chaveDeTeste, "gpt-antigo", "instruções", "Cliente: oi\n", esquemaSaida, 10*time.Second)
	if err != nil || !json.Valid(bruto) {
		t.Fatalf("devia tentar de novo sem o esquema: %v", err)
	}
	if len(o.pedidos) != 2 {
		t.Fatalf("esperava 2 pedidos, foram %d", len(o.pedidos))
	}
	segundo := o.pedidos[1]
	if segundo["response_format"].(map[string]any)["type"] != "json_object" {
		t.Errorf("a segunda tentativa devia pedir JSON simples: %v", segundo["response_format"])
	}
	if sistema := segundo["messages"].([]any)[0].(map[string]any)["content"].(string); !strings.Contains(sistema, "objeto JSON") {
		t.Errorf("a segunda tentativa precisa explicar o formato nas instruções")
	}
}

func TestFalhasDaOpenAIViramAvisosClaros(t *testing.T) {
	o := novaOpenAIDeMentira(t)
	casos := []struct{ recusar, motivo, trecho string }{
		{"chave", "openai_chave", "não aceitou a chave"},
		{"credito", "openai_credito", "sem crédito"},
		{"modelo", "openai_modelo", "modelo escolhido não existe"},
	}
	for _, caso := range casos {
		o.recusar = caso.recusar
		c := &Cerebro{}
		c.definirOpenAI(chaveDeTeste, "gpt-5-mini")
		antes := len(o.pedidos)
		_, _, err := c.Responder("", "instruções", "Cliente: oi\n", "oi")
		if err == nil {
			t.Fatalf("%s: devia falhar", caso.recusar)
		}
		motivo, aviso := motivoDaFalha(err.Error())
		if motivo != caso.motivo || !strings.Contains(aviso, caso.trecho) {
			t.Errorf("%s: motivo %q, aviso %q", caso.recusar, motivo, aviso)
		}
		if strings.Contains(err.Error(), "sk-") || strings.Contains(c.ultimoErro, "sk-") {
			t.Errorf("%s: o erro repete um pedaço da chave: %v", caso.recusar, err)
		}
		if caso.recusar != "chave" && len(o.pedidos)-antes != 1 {
			t.Errorf("%s: falha conhecida não deve ser tentada de novo (%d pedidos)", caso.recusar, len(o.pedidos)-antes)
		}
		w := httptest.NewRecorder()
		erroDaIA(w, err)
		if w.Code != http.StatusServiceUnavailable || !strings.Contains(w.Body.String(), `"conectar_ia":true`) || !strings.Contains(w.Body.String(), caso.trecho) {
			t.Errorf("%s: o painel devia explicar e abrir a conexão: %d %s", caso.recusar, w.Code, w.Body.String())
		}
	}
	if m, _ := motivoDaFalha(prefixoOpenAI + "server_error: The server had an error (código 500)"); m != "" {
		t.Errorf("falha passageira da OpenAI não tem motivo conhecido, veio %q", m)
	}
}

func TestChaveDaOpenAIPeloPainel(t *testing.T) {
	o := novaOpenAIDeMentira(t)
	t.Setenv("OPENAI_API_KEY", "")
	a := novoApp()
	cofre := a.cofre.(*cofreDeMemoria)

	// Chave mal copiada e chave que a OpenAI recusa: nada é guardado.
	if w := postar(a.rotaOpenAI, `{"chave":"sk curta"}`); w.Code != http.StatusBadRequest {
		t.Errorf("chave com espaço devia dar 400, deu %d", w.Code)
	}
	if w := postar(a.rotaOpenAI, `{"chave":"sk-outra-chave-que-a-openai-nao-conhece-0000"}`); w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "não aceitou esta chave") {
		t.Errorf("chave recusada: %d %s", w.Code, w.Body.String())
	}
	if v, _ := cofre.ler(chaveDaOpenAI); v != "" || a.cfg.Provedor == provedorOpenAI {
		t.Fatalf("chave ruim não pode ser guardada nem ativar o ChatGPT")
	}
	if w := postar(a.rotaProvedor, `{"provedor":"openai"}`); w.Code != http.StatusBadRequest {
		t.Errorf("sem chave não dá para escolher o ChatGPT: %d", w.Code)
	}

	// Chave boa: vai para o cofre, o ChatGPT passa a responder e a chave nunca volta para a página.
	w := postar(a.rotaOpenAI, `{"chave":"  `+chaveDeTeste+`  "}`)
	if w.Code != http.StatusOK {
		t.Fatalf("chave boa devia ser aceita: %d %s", w.Code, w.Body.String())
	}
	var r struct {
		Provedor  string   `json:"provedor"`
		Conectado bool     `json:"conectado"`
		Modelos   []string `json:"modelos"`
		OpenAI    struct {
			TemChave bool   `json:"tem_chave"`
			Final    string `json:"final"`
			Modelo   string `json:"modelo"`
			Origem   string `json:"origem"`
		} `json:"openai"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &r)
	if r.Provedor != "openai" || !r.Conectado || !r.OpenAI.TemChave || r.OpenAI.Final != "FIM9" || r.OpenAI.Modelo != "gpt-5-mini" || r.OpenAI.Origem != "cofre" {
		t.Errorf("depois de salvar a chave: %+v", r)
	}
	if strings.Join(r.Modelos, ",") != "gpt-4o-mini,gpt-5,gpt-5-mini" {
		t.Errorf("a lista devia ter só os modelos de conversa: %v", r.Modelos)
	}
	if strings.Contains(w.Body.String(), chaveDeTeste) {
		t.Errorf("a resposta da rota devolveu a chave")
	}
	if v, _ := cofre.ler(chaveDaOpenAI); v != chaveDeTeste {
		t.Errorf("a chave devia estar no cofre")
	}
	if cfg, _ := json.Marshal(a.cfg); strings.Contains(string(cfg), "sk-") || !strings.Contains(string(cfg), `"provedor":"openai"`) {
		t.Errorf("as configurações gravadas guardam o provedor, nunca a chave: %s", cfg)
	}
	for _, rota := range []http.HandlerFunc{a.rotaConexao, a.rotaEstado} {
		ww := httptest.NewRecorder()
		rota(ww, httptest.NewRequest(http.MethodGet, "/", nil))
		if strings.Contains(ww.Body.String(), "sk-") {
			t.Errorf("uma rota de leitura devolveu a chave")
		}
	}
	if s, modo, err := a.cerebro.Responder("claude-haiku-5-5", "instruções", "Cliente: oi\n", "oi"); err != nil || modo != "openai" || s.Resposta == "" {
		t.Errorf("o cérebro devia responder pelo ChatGPT: %q %v", modo, err)
	}

	// Trocar só o modelo, sem mandar a chave de novo.
	if w := postar(a.rotaOpenAI, `{"modelo":"gpt-5"}`); w.Code != http.StatusOK || a.cfg.ModeloOpenAI != "gpt-5" {
		t.Errorf("trocar o modelo: %d, modelo %q", w.Code, a.cfg.ModeloOpenAI)
	}
	if w := postar(a.rotaOpenAI, `{"modelo":"gpt 5; rm -rf"}`); w.Code != http.StatusBadRequest {
		t.Errorf("nome de modelo estranho devia dar 400, deu %d", w.Code)
	}

	// As configurações comuns do painel não mudam quem responde.
	req := httptest.NewRequest(http.MethodPost, "/api/config", strings.NewReader(`{"negocio":"Posto Exemplo","provedor":"claude","modelo_openai":"outro"}`))
	req.Header.Set("Content-Type", "application/json")
	a.rotaConfig(httptest.NewRecorder(), req)
	if a.cfg.Provedor != provedorOpenAI || a.cfg.ModeloOpenAI != "gpt-5" || a.cfg.Negocio != "Posto Exemplo" {
		t.Errorf("salvar o negócio não pode trocar o provedor: %q %q %q", a.cfg.Provedor, a.cfg.ModeloOpenAI, a.cfg.Negocio)
	}

	// Voltar para o Claude guarda a chave. Remover apaga do cofre.
	if w := postar(a.rotaProvedor, `{"provedor":"claude"}`); w.Code != http.StatusOK || a.cfg.Provedor != provedorClaude {
		t.Errorf("voltar para o Claude: %d %q", w.Code, a.cfg.Provedor)
	}
	if chave, _ := a.cerebro.viaOpenAI(); chave != "" {
		t.Errorf("com o Claude escolhido o cérebro não pode usar a OpenAI")
	}
	if v, _ := cofre.ler(chaveDaOpenAI); v != chaveDeTeste {
		t.Errorf("voltar para o Claude não apaga a chave")
	}
	antes := len(o.pedidos)
	postar(a.rotaProvedor, `{"provedor":"openai"}`)
	if w := postar(a.rotaRemoverOpenAI, `{}`); w.Code != http.StatusOK || a.cfg.Provedor != provedorClaude || a.chaveOpenAI != "" {
		t.Errorf("remover a chave: %d %q", w.Code, a.cfg.Provedor)
	}
	if v, _ := cofre.ler(chaveDaOpenAI); v != "" {
		t.Errorf("remover devia apagar a chave do cofre")
	}
	if len(o.pedidos) != antes {
		t.Errorf("trocar de provedor não devia gastar pedidos de conversa")
	}
}

func TestChaveDaOpenAISemCofreNoComputador(t *testing.T) {
	novaOpenAIDeMentira(t)
	t.Setenv("OPENAI_API_KEY", "")
	a := novoApp()
	a.cofre = &cofreDeMemoria{falha: errors.New("sem cofre neste computador")}
	w := postar(a.rotaOpenAI, `{"chave":"`+chaveDeTeste+`"}`)
	if w.Code != http.StatusInternalServerError || !strings.Contains(w.Body.String(), "não foi salva") {
		t.Errorf("sem cofre o painel avisa e não usa a chave: %d %s", w.Code, w.Body.String())
	}
	if a.chaveOpenAI != "" || a.cfg.Provedor == provedorOpenAI {
		t.Errorf("sem cofre a chave não pode ficar em uso")
	}

	// A variável de ambiente serve para esse computador. Ela não é apagada pelo painel.
	t.Setenv("OPENAI_API_KEY", chaveDeTeste)
	a.carregarChaveOpenAI()
	if a.chaveOpenAI != chaveDeTeste || a.origemOpenAI != "ambiente" {
		t.Fatalf("a chave do ambiente devia ser lida ao ligar")
	}
	if w := postar(a.rotaProvedor, `{"provedor":"openai"}`); w.Code != http.StatusOK {
		t.Errorf("com a chave do ambiente dá para escolher o ChatGPT: %d", w.Code)
	}
	if w := postar(a.rotaRemoverOpenAI, `{}`); w.Code != http.StatusConflict {
		t.Errorf("chave do ambiente não é apagada pelo painel: %d", w.Code)
	}
}

func TestChaveGuardadaVoltaAoLigar(t *testing.T) {
	novaOpenAIDeMentira(t)
	t.Setenv("OPENAI_API_KEY", "")
	a := novoApp()
	_ = a.cofre.guardar(chaveDaOpenAI, chaveDeTeste)
	a.cfg.Provedor, a.cfg.ModeloOpenAI = provedorOpenAI, "gpt-5-mini"
	a.carregarChaveOpenAI()
	if chave, modelo := a.cerebro.viaOpenAI(); chave != chaveDeTeste || modelo != "gpt-5-mini" || a.origemOpenAI != "cofre" {
		t.Errorf("ao ligar, a chave do cofre devia voltar para o cérebro")
	}
	s := a.situacaoDaIA(estadoIA{})
	if s.Provedor != provedorOpenAI || !s.Conectado || !s.Instalado {
		t.Errorf("com o ChatGPT em uso, a situação não depende do Claude: %+v", s)
	}
	a.cerebro.ultimoErro = errChaveRecusada.Error()
	if s := a.situacaoDaIA(estadoIA{}); s.Conectado || s.Motivo != "openai_chave" {
		t.Errorf("chave recusada numa resposta derruba a situação: %+v", s)
	}
}

func TestChatGPTSeAdaptaAoModelo(t *testing.T) {
	o := novaOpenAIDeMentira(t)

	// Modelo de raciocínio que só aceita as instruções no papel "developer".
	o.recusar = "papel"
	bruto, err := chamarOpenAI(chaveDeTeste, "o1-mini", "instruções", "Cliente: oi\n", esquemaSaida, 10*time.Second)
	if err != nil || !json.Valid(bruto) {
		t.Fatalf("devia tentar de novo com o papel developer: %v", err)
	}
	if n := len(o.pedidos); n != 2 || o.pedidos[1]["messages"].([]any)[0].(map[string]any)["role"] != "developer" {
		t.Errorf("segunda tentativa devia usar developer: %d pedidos", n)
	}

	// Resposta barrada pelo filtro de conteúdo: erro claro, sem quebrar.
	o.recusar = "filtro"
	if _, err := chamarOpenAI(chaveDeTeste, "gpt-5-mini", "instruções", "Cliente: oi\n", esquemaSaida, 10*time.Second); err == nil || !strings.Contains(err.Error(), "filtro de conteúdo") {
		t.Errorf("filtro de conteúdo: %v", err)
	}

	// Modelo que só existe na outra rota da OpenAI vira o aviso de trocar o modelo.
	for _, msg := range []string{
		"openai: invalid_request_error: This model is only supported in v1/responses and not in v1/chat/completions. (código 404)",
		"openai: model_not_found: The model `gpt-x` does not exist or you do not have access to it. (código 404)",
	} {
		if m, _ := motivoDaFalha(msg); m != "openai_modelo" {
			t.Errorf("%q devia pedir a troca do modelo, veio %q", msg, m)
		}
	}
	for id, quer := range map[string]bool{
		"gpt-5-mini": true, "gpt-4o": true, "gpt-6-astra": true, "gpt-5-chat-latest": true,
		"gpt-5-pro": false, "gpt-5-pro-2026-01-01": false, "gpt-4o-mini-tts": false, "gpt-image-1": false,
		"gpt-4o-transcribe": false, "gpt-realtime": false, "o3-deep-research": false, "text-embedding-3-small": false,
	} {
		if modeloDeConversa(id) != quer {
			t.Errorf("modeloDeConversa(%q) devia ser %v", id, quer)
		}
	}
}

func TestChaveRestritaSemListaDeModelos(t *testing.T) {
	o := novaOpenAIDeMentira(t)
	o.recusar = "listar"
	t.Setenv("OPENAI_API_KEY", "")
	a := novoApp()
	w := postar(a.rotaOpenAI, `{"chave":"`+chaveDeTeste+`"}`)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"modelos":[]`) {
		t.Fatalf("chave sem permissão de listar modelos ainda deve ser aceita: %d %s", w.Code, w.Body.String())
	}
	if a.cfg.Provedor != provedorOpenAI || a.cfg.ModeloOpenAI != preferidosOpenAI[0] {
		t.Errorf("sem lista, fica o modelo padrão: %q %q", a.cfg.Provedor, a.cfg.ModeloOpenAI)
	}
	if w := postar(a.rotaOpenAI, `{"modelo":"gpt-6-astra"}`); w.Code != http.StatusOK || a.cfg.ModeloOpenAI != "gpt-6-astra" {
		t.Errorf("a pessoa pode escrever o modelo: %d %q", w.Code, a.cfg.ModeloOpenAI)
	}
}

// Regras do modo estrito da OpenAI (guia Structured Outputs): raiz objeto, todo campo em
// "required", additionalProperties falso em todo objeto e só palavras aceitas.
func TestEsquemasSeguemOModoEstritoDaOpenAI(t *testing.T) {
	aceitas := map[string]bool{"type": true, "properties": true, "required": true, "items": true, "enum": true, "additionalProperties": true, "description": true}
	var conferir func(nome, caminho string, v any)
	conferir = func(nome, caminho string, v any) {
		m, ok := v.(map[string]any)
		if !ok {
			t.Errorf("%s %s: esperava um objeto de esquema", nome, caminho)
			return
		}
		for k := range m {
			if !aceitas[k] {
				t.Errorf("%s %s: a palavra %q pode não ser aceita no modo estrito", nome, caminho, k)
			}
		}
		switch m["type"] {
		case "object":
			if m["additionalProperties"] != false {
				t.Errorf("%s %s: objeto sem additionalProperties falso", nome, caminho)
			}
			exigidos := map[string]bool{}
			for _, r := range m["required"].([]any) {
				exigidos[r.(string)] = true
			}
			for k, p := range m["properties"].(map[string]any) {
				if !exigidos[k] {
					t.Errorf("%s %s: o campo %q precisa estar em required", nome, caminho, k)
				}
				conferir(nome, caminho+"."+k, p)
			}
		case "array":
			conferir(nome, caminho+"[]", m["items"])
		}
	}
	for nome, esquema := range map[string]string{
		"saida": esquemaSaida, "negocio": esquemaNegocio, "melhoria": esquemaMelhoria, "texto": esquemaTexto, "material": esquemaMaterial,
	} {
		var raiz map[string]any
		if err := json.Unmarshal(comAdicionaisFalsos(esquema), &raiz); err != nil || raiz["type"] != "object" {
			t.Errorf("%s: a raiz precisa ser um objeto: %v", nome, err)
			continue
		}
		conferir(nome, "raiz", raiz)
	}
}

// Fala com o servidor de verdade da OpenAI usando uma chave inventada. Não gasta nada e
// não manda nenhum dado. Só roda com ATENDENTE_TESTE_REDE=1, porque precisa de internet.
func TestOpenAIDeVerdadeRecusaChaveInventada(t *testing.T) {
	if os.Getenv("ATENDENTE_TESTE_REDE") != "1" {
		t.Skip("defina ATENDENTE_TESTE_REDE=1 para falar com a OpenAI de verdade")
	}
	const inventada = "sk-chave-inventada-para-teste-0000000000"
	if _, err := modelosDaOpenAI(inventada); !errors.Is(err, errChaveRecusada) {
		t.Errorf("listar modelos com chave inventada devia dar chave recusada, deu: %v", err)
	}
	_, err := chamarOpenAI(inventada, "gpt-4o-mini", "teste", "teste", esquemaTexto, 30*time.Second)
	if !errors.Is(err, errChaveRecusada) {
		t.Errorf("pedir resposta com chave inventada devia dar chave recusada, deu: %v", err)
	}
	if motivo, _ := motivoDaFalha(err.Error()); motivo != "openai_chave" || strings.Contains(err.Error(), "sk-") {
		t.Errorf("o aviso devia ser o de chave recusada, sem repetir a chave: %q %v", motivo, err)
	}
}
