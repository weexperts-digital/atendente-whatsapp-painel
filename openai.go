package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"
)

// ChatGPT pela chave da OpenAI: a segunda opcao de cerebro.
//
// O pedido leva so texto (instrucoes e conversa) e pede a resposta no formato do
// esquema. Nao declara ferramenta nenhuma, entao o modelo nao tem como executar
// comandos, abrir arquivos ou acessar a internet. E a mesma regra do claude sem ferramentas.

// enderecoOpenAI e variavel so para os testes apontarem para um servidor de mentira.
var enderecoOpenAI = "https://api.openai.com/v1"

const prefixoOpenAI = "openai: " // comeco de todo erro que vem da OpenAI

var (
	errChaveRecusada = errors.New(prefixoOpenAI + "invalid_api_key: a OpenAI não aceitou a chave")
	// preferidosOpenAI: em ordem, os modelos escolhidos quando a pessoa nao escolhe.
	// Vale o primeiro que existir na conta. Se nenhum existir, vale um "mini" qualquer.
	preferidosOpenAI = []string{"gpt-5-mini", "gpt-5.1-mini", "gpt-4.1-mini", "gpt-4o-mini"}
)

// erroDaOpenAI le a resposta de erro sem nunca repetir a chave (a OpenAI devolve um
// pedaco dela na mensagem de chave errada).
func erroDaOpenAI(status int, dados []byte) error {
	var r struct {
		Error struct {
			Message string `json:"message"`
			Type    string `json:"type"`
			Code    any    `json:"code"`
		} `json:"error"`
	}
	_ = json.Unmarshal(dados, &r)
	codigo, _ := r.Error.Code.(string)
	if codigo == "" {
		codigo = r.Error.Type
	}
	if status == http.StatusUnauthorized || codigo == "invalid_api_key" {
		return errChaveRecusada
	}
	msg := r.Error.Message
	if msg == "" {
		msg = cortar(string(dados), 200)
	}
	return fmt.Errorf("%s%s: %s (código %d)", prefixoOpenAI, ouPadrao(codigo, "erro"), cortar(umaLinha(msg), 240), status)
}

func pedirOpenAI(ctx context.Context, metodo, caminho, chave string, corpo []byte) (int, []byte, error) {
	var leitor io.Reader
	if corpo != nil {
		leitor = bytes.NewReader(corpo)
	}
	req, err := http.NewRequestWithContext(ctx, metodo, enderecoOpenAI+caminho, leitor)
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Authorization", "Bearer "+chave)
	if corpo != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return 0, nil, errors.New(prefixoOpenAI + "demorou demais para responder")
		}
		return 0, nil, errors.New(prefixoOpenAI + "sem resposta. Confira a internet deste computador")
	}
	defer resp.Body.Close()
	dados, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	return resp.StatusCode, dados, err
}

// chamarOpenAI pede uma resposta no formato do esquema. Se o modelo nao aceitar o
// esquema, tenta de novo pedindo um JSON simples, com o esquema escrito nas instrucoes.
func chamarOpenAI(chave, modelo, sistema, entrada, esquema string, limite time.Duration) (json.RawMessage, error) {
	papel, estruturado := "system", true
	// No maximo 3 pedidos: o normal e uma nova tentativa para cada ajuste abaixo.
	for i := 0; i < 3; i++ {
		bruto, status, err := postarOpenAI(chave, modelo, papel, sistema, entrada, esquema, estruturado, limite)
		if err == nil || status != http.StatusBadRequest {
			return bruto, err
		}
		msg := err.Error()
		switch {
		case papel == "system" && strings.Contains(msg, "'system'"):
			// Alguns modelos de raciocinio so aceitam as instrucoes no papel "developer".
			papel = "developer"
		case estruturado && (strings.Contains(msg, "response_format") || strings.Contains(msg, "schema")):
			// Modelo antigo, sem saida pelo esquema: pede um JSON simples. A documentacao
			// exige a palavra JSON nas mensagens, e ela vai na instrucao.
			estruturado = false
			sistema += "\n\nResponda SOMENTE com um objeto JSON válido, sem texto antes ou depois, seguindo este esquema: " + esquema + "\n"
		default:
			return nil, err
		}
	}
	return nil, errors.New(prefixoOpenAI + "o modelo não aceitou o formato do pedido")
}

func postarOpenAI(chave, modelo, papel, sistema, entrada, esquema string, estruturado bool, limite time.Duration) (json.RawMessage, int, error) {
	formato := map[string]any{"type": "json_object"}
	if estruturado {
		formato = map[string]any{
			"type":        "json_schema",
			"json_schema": map[string]any{"name": "saida", "strict": true, "schema": comAdicionaisFalsos(esquema)},
		}
	}
	corpo, err := json.Marshal(map[string]any{
		"model": modelo,
		"messages": []map[string]string{
			{"role": papel, "content": sistema},
			{"role": "user", "content": entrada},
		},
		"response_format": formato,
	})
	if err != nil {
		return nil, 0, err
	}
	ctx, cancelar := context.WithTimeout(context.Background(), limite)
	defer cancelar()
	status, dados, err := pedirOpenAI(ctx, http.MethodPost, "/chat/completions", chave, corpo)
	if err != nil {
		return nil, status, err
	}
	if status != http.StatusOK {
		return nil, status, erroDaOpenAI(status, dados)
	}
	var r struct {
		Choices []struct {
			FinishReason string `json:"finish_reason"`
			Message      struct {
				Content string `json:"content"`
				Refusal string `json:"refusal"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(dados, &r); err != nil || len(r.Choices) == 0 {
		return nil, status, errors.New(prefixoOpenAI + "resposta fora do formato")
	}
	escolha := r.Choices[0]
	if escolha.Message.Refusal != "" {
		return nil, status, errors.New(prefixoOpenAI + "o modelo se recusou a responder")
	}
	switch escolha.FinishReason {
	case "length":
		return nil, status, errors.New(prefixoOpenAI + "a resposta veio cortada")
	case "content_filter":
		return nil, status, errors.New(prefixoOpenAI + "a resposta foi barrada pelo filtro de conteúdo")
	}
	texto := extrairJSON(escolha.Message.Content)
	if !json.Valid([]byte(texto)) {
		return nil, status, errors.New(prefixoOpenAI + "resposta fora do formato")
	}
	return json.RawMessage(texto), status, nil
}

// modelosDaOpenAI lista os modelos de conversa da conta. Serve tambem para conferir a
// chave: e uma consulta que nao gasta credito.
func modelosDaOpenAI(chave string) ([]string, error) {
	ctx, cancelar := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancelar()
	status, dados, err := pedirOpenAI(ctx, http.MethodGet, "/models", chave, nil)
	if err != nil {
		return nil, err
	}
	if status == http.StatusForbidden {
		// Chave restrita, sem permissao de listar modelos. Ela pode responder mesmo
		// assim: a lista fica vazia e a pessoa escreve o nome do modelo.
		return nil, nil
	}
	if status != http.StatusOK {
		return nil, erroDaOpenAI(status, dados)
	}
	var r struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(dados, &r); err != nil {
		return nil, errors.New(prefixoOpenAI + "resposta fora do formato")
	}
	var lista []string
	for _, m := range r.Data {
		if modeloDeConversa(m.ID) {
			lista = append(lista, m.ID)
		}
	}
	sort.Strings(lista)
	return lista, nil
}

// modeloDeConversa separa os modelos de texto dos de voz, imagem e outros usos.
func modeloDeConversa(id string) bool {
	if !strings.HasPrefix(id, "gpt-") || len(id) > 60 {
		return false
	}
	for _, fora := range []string{"audio", "realtime", "transcribe", "tts", "image", "search", "embedding", "moderation", "instruct", "codex", "vision", "deep-research", "-pro-"} {
		if strings.Contains(id, fora) {
			return false
		}
	}
	// Os modelos "pro" so funcionam em outra rota da OpenAI, que o atendente nao usa.
	return !strings.HasSuffix(id, "-pro")
}

// modeloOpenAIPadrao escolhe um modelo rapido e barato entre os que a conta tem.
func modeloOpenAIPadrao(lista []string) string {
	tem := map[string]bool{}
	for _, m := range lista {
		tem[m] = true
	}
	for _, m := range preferidosOpenAI {
		if tem[m] {
			return m
		}
	}
	for _, m := range lista {
		if strings.HasSuffix(m, "-mini") {
			return m
		}
	}
	if len(lista) > 0 {
		return lista[0]
	}
	return preferidosOpenAI[0]
}

// modeloOpenAILimpo aceita so o que tem cara de nome de modelo.
func modeloOpenAILimpo(s string) string {
	s = strings.TrimSpace(s)
	if s == "" || len(s) > 60 {
		return ""
	}
	for _, r := range s {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '.' || r == '_' || r == ':') {
			return ""
		}
	}
	return s
}

// chaveOpenAILimpa confere so o formato: sem espacos, sem quebra de linha, tamanho de chave.
func chaveOpenAILimpa(s string) string {
	s = strings.TrimSpace(s)
	if len(s) < 20 || len(s) > 400 {
		return ""
	}
	for _, r := range s {
		if r <= ' ' || r > '~' {
			return ""
		}
	}
	return s
}

// finalDaChave mostra so as 4 ultimas letras, para a pessoa reconhecer a chave.
func finalDaChave(chave string) string {
	if len(chave) < 12 {
		return ""
	}
	return chave[len(chave)-4:]
}

// carregarChaveOpenAI le a chave ao ligar: primeiro a variavel de ambiente
// OPENAI_API_KEY (para computador sem cofre de senhas), depois o cofre do sistema.
func (a *App) carregarChaveOpenAI() {
	chave, origem := chaveOpenAILimpa(os.Getenv("OPENAI_API_KEY")), "ambiente"
	if chave == "" {
		origem = ""
		if guardada, err := a.cofre.ler(chaveDaOpenAI); err != nil {
			log.Printf("IA: não consegui ler o cofre de senhas do sistema: %v", err)
		} else if guardada = chaveOpenAILimpa(guardada); guardada != "" {
			chave, origem = guardada, "cofre"
		}
	}
	a.mu.Lock()
	a.chaveOpenAI, a.origemOpenAI = chave, origem
	a.aplicarProvedor()
	a.mu.Unlock()
}

// aplicarProvedor avisa o cerebro de quem responde agora. Chame com a.mu travado.
func (a *App) aplicarProvedor() {
	if a.cfg.Provedor == provedorOpenAI && a.chaveOpenAI != "" {
		a.cerebro.definirOpenAI(a.chaveOpenAI, ouPadrao(a.cfg.ModeloOpenAI, preferidosOpenAI[0]))
		return
	}
	a.cerebro.definirOpenAI("", "")
}

// visaoOpenAI e o que o painel sabe da chave: se existe e como termina. Nunca a chave.
func (a *App) visaoOpenAI() map[string]any {
	a.mu.Lock()
	defer a.mu.Unlock()
	return map[string]any{
		"tem_chave": a.chaveOpenAI != "",
		"final":     finalDaChave(a.chaveOpenAI),
		"modelo":    a.cfg.ModeloOpenAI,
		"origem":    a.origemOpenAI,
	}
}

// rotaOpenAI guarda a chave (depois de conferir com a OpenAI), escolhe o modelo e passa
// a usar o ChatGPT. Sem "chave" no pedido, so troca o modelo.
func (a *App) rotaOpenAI(w http.ResponseWriter, r *http.Request) {
	var p struct {
		Chave  string `json:"chave"`
		Modelo string `json:"modelo"`
	}
	if !lerPedido(w, r, &p) {
		return
	}
	a.mu.Lock()
	chave, origem := a.chaveOpenAI, a.origemOpenAI
	a.mu.Unlock()

	nova := strings.TrimSpace(p.Chave) != ""
	if nova {
		chave = chaveOpenAILimpa(p.Chave)
		if chave == "" {
			responderErro(w, http.StatusBadRequest, "Isso não parece uma chave da OpenAI. Copie a chave inteira, sem espaços. Ela costuma começar com sk-.")
			return
		}
	}
	if chave == "" {
		responderErro(w, http.StatusBadRequest, "Cole a chave da OpenAI para usar o ChatGPT.")
		return
	}
	modelos, err := modelosDaOpenAI(chave)
	if err != nil {
		if errors.Is(err, errChaveRecusada) {
			responderErro(w, http.StatusBadRequest, "A OpenAI não aceitou esta chave. Confira se copiou a chave inteira ou crie uma nova.")
			return
		}
		log.Printf("IA: não consegui conferir a chave com a OpenAI: %v", err)
		responderErro(w, http.StatusBadGateway, "Não consegui falar com a OpenAI para conferir a chave. Confira a internet e tente de novo.")
		return
	}
	modelo := modeloOpenAILimpo(p.Modelo)
	if strings.TrimSpace(p.Modelo) != "" && modelo == "" {
		responderErro(w, http.StatusBadRequest, "O nome do modelo não parece válido.")
		return
	}
	if nova {
		if err := a.cofre.guardar(chaveDaOpenAI, chave); err != nil {
			log.Printf("IA: não consegui guardar a chave no cofre do sistema: %v", err)
			responderErro(w, http.StatusInternalServerError, "Não consegui guardar a chave no cofre de senhas deste computador. A chave não foi salva.")
			return
		}
		origem = "cofre"
	}

	a.mu.Lock()
	if modelo == "" {
		modelo = ouPadrao(a.cfg.ModeloOpenAI, modeloOpenAIPadrao(modelos))
	}
	a.chaveOpenAI, a.origemOpenAI = chave, origem
	a.cfg.Provedor, a.cfg.ModeloOpenAI = provedorOpenAI, modelo
	a.aplicarProvedor()
	a.salvarConfig()
	a.mu.Unlock()
	a.cerebro.esquecerFalha()
	a.registrar(usuarioDe(r), "ia_openai_ativada", modelo)
	log.Printf("IA: o assistente passou a usar o ChatGPT (%s)", modelo)

	if modelos == nil {
		modelos = []string{} // chave restrita, sem permissão de listar: a pessoa escreve o modelo
	}
	v := a.visaoDaConexao(false)
	v["modelos"] = modelos
	responderJSON(w, http.StatusOK, v)
}

// rotaModelosOpenAI lista os modelos da conta, para a pessoa escolher.
func (a *App) rotaModelosOpenAI(w http.ResponseWriter, r *http.Request) {
	a.mu.Lock()
	chave := a.chaveOpenAI
	a.mu.Unlock()
	if chave == "" {
		responderJSON(w, http.StatusOK, map[string]any{"modelos": []string{}})
		return
	}
	modelos, err := modelosDaOpenAI(chave)
	if err != nil {
		log.Printf("IA: não consegui listar os modelos da OpenAI: %v", err)
		responderErro(w, http.StatusBadGateway, ouPadrao(avisoDoCerebro(err.Error()), "Não consegui buscar os modelos da OpenAI. Tente de novo."))
		return
	}
	if modelos == nil {
		modelos = []string{}
	}
	responderJSON(w, http.StatusOK, map[string]any{"modelos": modelos})
}

// rotaProvedor troca quem responde: o Claude ou o ChatGPT.
func (a *App) rotaProvedor(w http.ResponseWriter, r *http.Request) {
	var p struct {
		Provedor string `json:"provedor"`
	}
	if !lerPedido(w, r, &p) {
		return
	}
	if p.Provedor != provedorClaude && p.Provedor != provedorOpenAI {
		responderErro(w, http.StatusBadRequest, "Escolha o Claude ou o ChatGPT.")
		return
	}
	a.mu.Lock()
	if p.Provedor == provedorOpenAI && a.chaveOpenAI == "" {
		a.mu.Unlock()
		responderErro(w, http.StatusBadRequest, "Para usar o ChatGPT, cole primeiro a chave da OpenAI.")
		return
	}
	a.cfg.Provedor = p.Provedor
	a.aplicarProvedor()
	a.salvarConfig()
	a.mu.Unlock()
	a.cerebro.esquecerFalha()
	acao := "ia_claude_ativado"
	if p.Provedor == provedorOpenAI {
		acao = "ia_openai_ativada"
	}
	a.registrar(usuarioDe(r), acao, "")
	responderJSON(w, http.StatusOK, a.visaoDaConexao(false))
}

// rotaRemoverOpenAI apaga a chave do cofre e volta para o Claude.
func (a *App) rotaRemoverOpenAI(w http.ResponseWriter, r *http.Request) {
	if !lerPedido(w, r, &struct{}{}) {
		return
	}
	a.mu.Lock()
	origem := a.origemOpenAI
	a.mu.Unlock()
	if origem == "ambiente" {
		responderErro(w, http.StatusConflict, "Esta chave vem da variável OPENAI_API_KEY, definida ao ligar o atendente. Ela não fica guardada aqui, então não há o que apagar.")
		return
	}
	if err := a.cofre.apagar(chaveDaOpenAI); err != nil {
		log.Printf("IA: não consegui apagar a chave do cofre do sistema: %v", err)
		responderErro(w, http.StatusInternalServerError, "Não consegui apagar a chave do cofre de senhas deste computador.")
		return
	}
	a.mu.Lock()
	a.chaveOpenAI, a.origemOpenAI = "", ""
	a.cfg.Provedor = provedorClaude
	a.aplicarProvedor()
	a.salvarConfig()
	a.mu.Unlock()
	a.cerebro.esquecerFalha()
	a.registrar(usuarioDe(r), "ia_chave_removida", "")
	responderJSON(w, http.StatusOK, a.visaoDaConexao(false))
}
