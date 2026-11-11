package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

// O cerebro e o claude em modo headless, SEM ferramentas. Nenhuma mensagem de
// WhatsApp consegue executar comandos, abrir arquivos ou acessar a internet.

const esquemaSaida = `{"type":"object","properties":{"resposta":{"type":"string"},"transbordo":{"type":"boolean"},"motivo":{"type":"string"},"lead":{"type":"object","properties":{"nome":{"type":"string"},"necessidade":{"type":"string"},"orcamento":{"type":"string"},"decisor":{"type":"string"},"prazo":{"type":"string"},"pontuacao":{"type":"integer"},"classificacao":{"type":"string","enum":["quente","morno","frio","indefinido"]},"etapa":{"type":"string"},"resumo":{"type":"string"}},"required":["nome","necessidade","orcamento","decisor","prazo","pontuacao","classificacao","etapa","resumo"]},"agendamento":{"type":"object","properties":{"acao":{"type":"string","enum":["nenhuma","marcar","cancelar"]},"servico":{"type":"string"},"data":{"type":"string"},"hora":{"type":"string"}},"required":["acao","servico","data","hora"]}},"required":["resposta","transbordo","motivo","lead","agendamento"]}`

const (
	respostaFalha  = "Só um instante, por favor. Estou transferindo o seu atendimento para a nossa equipe, que continua com você por aqui."
	limiteResposta = 90 * time.Second
	limiteAjudante = 170 * time.Second
)

var errSemCerebro = errors.New("esta função precisa do Claude. Instale o Claude Code e entre na sua conta")

// numFlex aceita 70, 70.0 ou "70".
type numFlex int

func (n *numFlex) UnmarshalJSON(b []byte) error {
	s := strings.Trim(strings.TrimSpace(string(b)), `"`)
	if s == "" || s == "null" {
		*n = 0
		return nil
	}
	f, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil {
		*n = 0
		return nil
	}
	*n = numFlex(math.Round(f))
	return nil
}

// LeadIA e o que o cerebro devolve sobre a pessoa.
type LeadIA struct {
	Nome          string  `json:"nome"`
	Necessidade   string  `json:"necessidade"`
	Orcamento     string  `json:"orcamento"`
	Decisor       string  `json:"decisor"`
	Prazo         string  `json:"prazo"`
	Pontuacao     numFlex `json:"pontuacao"`
	Classificacao string  `json:"classificacao"`
	Etapa         string  `json:"etapa"`
	Resumo        string  `json:"resumo"`
}

// AgendamentoIA e o pedido de agenda que o cerebro faz.
type AgendamentoIA struct {
	Acao    string `json:"acao"`
	Servico string `json:"servico"`
	Data    string `json:"data"`
	Hora    string `json:"hora"`
}

// Saida e a resposta completa do cerebro.
type Saida struct {
	Resposta    string        `json:"resposta"`
	Transbordo  bool          `json:"transbordo"`
	Motivo      string        `json:"motivo"`
	Lead        LeadIA        `json:"lead"`
	Agendamento AgendamentoIA `json:"agendamento"`
}

// Cerebro lembra das falhas para decidir quando usar a reserva.
type Cerebro struct {
	mu          sync.Mutex
	falhas      int
	ultimaFalha time.Time
	promptTexto bool // usar --system-prompt em vez de --system-prompt-file
	ultimoErro  string
}

// acharClaude encontra o programa claude mesmo quando o app roda em segundo plano.
func acharClaude() string {
	if v := strings.TrimSpace(os.Getenv("CLAUDE_BIN")); v != "" && existeArquivo(v) {
		return v
	}
	if p, err := exec.LookPath("claude"); err == nil {
		return p
	}
	casa, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	nome := "claude"
	if runtime.GOOS == "windows" {
		nome = "claude.exe"
	}
	p := filepath.Join(casa, ".local", "bin", nome)
	if existeArquivo(p) {
		return p
	}
	return ""
}

// modeloDe escolhe o modelo das respostas: variavel de ambiente, depois o painel.
func modeloDe(cfg Config) string {
	if v := strings.TrimSpace(os.Getenv("CLAUDE_MODELO")); v != "" {
		return v
	}
	return ouPadrao(cfg.Modelo, modeloPadrao)
}

func modeloDoAjudante() string {
	if v := strings.TrimSpace(os.Getenv("CLAUDE_MODELO_AJUDANTE")); v != "" {
		return v
	}
	return modeloAjudante
}

// ambienteFiltrado tira o que atrapalha o claude filho: a chave de API (para usar a
// assinatura da pessoa) e as variaveis de sessao do Claude Code (sessao dentro de sessao).
func ambienteFiltrado() []string {
	var saida []string
	for _, kv := range os.Environ() {
		nome := kv
		if i := strings.IndexByte(kv, '='); i >= 0 {
			nome = kv[:i]
		}
		n := strings.ToUpper(nome)
		if n == "ANTHROPIC_API_KEY" || n == "CLAUDECODE" {
			continue
		}
		if strings.HasPrefix(n, "CLAUDE_CODE_") && n != "CLAUDE_CODE_OAUTH_TOKEN" {
			continue
		}
		saida = append(saida, kv)
	}
	return saida
}

func (c *Cerebro) emReserva(temChave bool) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return temChave && c.falhas >= 2 && time.Since(c.ultimaFalha) < 10*time.Minute
}

// Modo diz, para a pagina, qual cerebro esta em uso agora.
func (c *Cerebro) Modo() (modo, erro string) {
	bin := acharClaude()
	temChave := os.Getenv("ANTHROPIC_API_KEY") != ""
	c.mu.Lock()
	erro = c.ultimoErro
	c.mu.Unlock()
	switch {
	case bin == "" && !temChave:
		return "teste", erro
	case bin == "":
		return "api", erro
	case c.emReserva(temChave):
		return "api", erro
	default:
		return "claude", erro
	}
}

// pedir chama o claude, sem ferramentas, e devolve o JSON no formato do esquema.
// Se o claude faltar ou falhar duas vezes seguidas e houver ANTHROPIC_API_KEY,
// usa a Messages API como reserva.
//
// principal e verdadeiro para as respostas aos clientes: tenta duas vezes e conta as
// falhas que decidem a reserva. Os pedidos de ajuda (rascunhos, resumos) tentam uma
// vez so e nao mexem nessa conta.
func (c *Cerebro) pedir(modelo, sistema, entrada, esquema string, limite time.Duration, principal bool) (json.RawMessage, string, error) {
	bin := acharClaude()
	chave := os.Getenv("ANTHROPIC_API_KEY")
	if bin == "" && chave == "" {
		return nil, "teste", errSemCerebro
	}

	var erroClaude error
	if bin != "" && !c.emReserva(chave != "") {
		c.mu.Lock()
		tentativas := 2
		if c.falhas >= 2 || !principal {
			tentativas = 1
		}
		c.mu.Unlock()
		for i := 0; i < tentativas; i++ {
			bruto, err := c.chamarClaude(bin, modelo, sistema, entrada, esquema, limite)
			if err == nil {
				if principal {
					c.mu.Lock()
					c.falhas = 0
					c.ultimoErro = ""
					c.mu.Unlock()
				}
				return bruto, "claude", nil
			}
			erroClaude = err
			if principal {
				c.mu.Lock()
				c.falhas++
				c.ultimaFalha = time.Now()
				c.ultimoErro = cortar(err.Error(), 300)
				c.mu.Unlock()
			}
			log.Printf("CEREBRO: o claude falhou (tentativa %d): %v", i+1, err)
		}
	}

	if chave != "" {
		bruto, err := chamarAPI(chave, modelo, sistema, entrada, esquema, limite)
		if err == nil {
			return bruto, "api", nil
		}
		log.Printf("CEREBRO: a API falhou: %v", err)
		if principal {
			c.mu.Lock()
			c.ultimoErro = cortar(err.Error(), 300)
			c.mu.Unlock()
		}
		if erroClaude != nil {
			return nil, "api", fmt.Errorf("claude: %v; API: %w", erroClaude, err)
		}
		return nil, "api", err
	}
	if erroClaude == nil {
		erroClaude = errors.New("o claude não está disponível")
	}
	return nil, "claude", erroClaude
}

// Responder passa a conversa pelo cerebro e devolve a resposta, o modo usado e o erro.
func (c *Cerebro) Responder(modelo, sistema, entrada, textoNovo string) (Saida, string, error) {
	if acharClaude() == "" && os.Getenv("ANTHROPIC_API_KEY") == "" {
		return modoTeste(textoNovo), "teste", nil
	}
	bruto, modo, err := c.pedir(modelo, sistema, entrada, esquemaSaida, limiteResposta, true)
	if err != nil {
		return Saida{}, modo, err
	}
	s, err := lerSaida(bruto)
	return s, modo, err
}

func (c *Cerebro) chamarClaude(bin, modelo, sistema, entrada, esquema string, limite time.Duration) (json.RawMessage, error) {
	c.mu.Lock()
	comoTexto := c.promptTexto
	c.mu.Unlock()

	bruto, diag, err := executarClaude(bin, modelo, sistema, entrada, esquema, comoTexto, limite)
	if err != nil && !comoTexto && strings.Contains(diag, "system-prompt-file") {
		// Versao do claude sem --system-prompt-file: passa o texto direto.
		c.mu.Lock()
		c.promptTexto = true
		c.mu.Unlock()
		bruto, _, err = executarClaude(bin, modelo, sistema, entrada, esquema, true, limite)
	}
	return bruto, err
}

// executarClaude roda o claude sem shell, sem ferramentas, em uma pasta vazia.
func executarClaude(bin, modelo, sistema, entrada, esquema string, comoTexto bool, limite time.Duration) (json.RawMessage, string, error) {
	pastaVazia, err := os.MkdirTemp("", "atendente-vazio-")
	if err != nil {
		return nil, "", err
	}
	defer os.RemoveAll(pastaVazia)

	args := []string{
		"-p",
		"--model", modelo,
		"--tools", "",
		"--strict-mcp-config",
		"--disable-slash-commands",
		"--no-session-persistence",
		"--setting-sources", "",
	}
	if comoTexto {
		args = append(args, "--system-prompt", sistema)
	} else {
		arq, err := os.CreateTemp("", "atendente-instrucoes-*.md")
		if err != nil {
			return nil, "", err
		}
		nome := arq.Name()
		defer os.Remove(nome)
		if _, err := arq.WriteString(sistema); err != nil {
			arq.Close()
			return nil, "", err
		}
		if err := arq.Close(); err != nil {
			return nil, "", err
		}
		args = append(args, "--system-prompt-file", nome)
	}
	args = append(args, "--output-format", "json", "--json-schema", esquema)

	ctx, cancelar := context.WithTimeout(context.Background(), limite)
	defer cancelar()
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Dir = pastaVazia
	cmd.Env = ambienteFiltrado()
	cmd.Stdin = strings.NewReader(entrada)
	cmd.WaitDelay = 5 * time.Second
	var saida, erros bytes.Buffer
	cmd.Stdout = &saida
	cmd.Stderr = &erros
	errExec := cmd.Run()

	diag := cortar(strings.TrimSpace(erros.String()), 600)
	if ctx.Err() == context.DeadlineExceeded {
		return nil, diag, fmt.Errorf("o claude demorou mais de %d segundos", int(limite.Seconds()))
	}
	bruto := bytes.TrimSpace(saida.Bytes())
	if len(bruto) == 0 {
		if errExec != nil {
			return nil, diag, fmt.Errorf("o claude terminou com erro: %v. %s", errExec, diag)
		}
		return nil, diag, errors.New("o claude não devolveu nada")
	}

	var resp struct {
		IsError          bool            `json:"is_error"`
		Result           string          `json:"result"`
		StructuredOutput json.RawMessage `json:"structured_output"`
	}
	if err := json.Unmarshal(bruto, &resp); err != nil {
		return nil, diag + " " + cortar(string(bruto), 300), fmt.Errorf("saída do claude não é JSON: %s", cortar(string(bruto), 200))
	}
	if resp.IsError {
		return nil, diag + " " + cortar(resp.Result, 300), fmt.Errorf("o claude avisou um erro: %s", cortar(resp.Result, 200))
	}
	estruturado := bytes.TrimSpace(resp.StructuredOutput)
	if len(estruturado) == 0 || string(estruturado) == "null" {
		// Sem structured_output: tenta ler o texto como JSON antes de desistir.
		if j := extrairJSON(resp.Result); json.Valid([]byte(j)) {
			return json.RawMessage(j), diag, nil
		}
		return nil, diag, errors.New("o claude não devolveu structured_output")
	}
	return json.RawMessage(estruturado), diag, nil
}

func lerSaida(b []byte) (Saida, error) {
	var s Saida
	if err := json.Unmarshal(b, &s); err != nil {
		return Saida{}, fmt.Errorf("resposta fora do formato: %v", err)
	}
	s.Resposta = strings.TrimSpace(s.Resposta)
	if s.Resposta == "" {
		return Saida{}, errors.New("o cérebro devolveu uma resposta vazia")
	}
	if strings.TrimSpace(s.Agendamento.Acao) == "" {
		s.Agendamento.Acao = "nenhuma"
	}
	return s, nil
}

// extrairJSON pega o trecho entre a primeira { e a ultima }.
func extrairJSON(t string) string {
	i := strings.IndexByte(t, '{')
	j := strings.LastIndexByte(t, '}')
	if i < 0 || j <= i {
		return ""
	}
	return t[i : j+1]
}

// comAdicionaisFalsos devolve o esquema com additionalProperties falso em todos os
// objetos, que e o que a Messages API exige para saida estruturada.
func comAdicionaisFalsos(esquema string) json.RawMessage {
	var raiz any
	if err := json.Unmarshal([]byte(esquema), &raiz); err != nil {
		return json.RawMessage(esquema)
	}
	var andar func(v any)
	andar = func(v any) {
		m, ok := v.(map[string]any)
		if !ok {
			return
		}
		if m["type"] == "object" {
			m["additionalProperties"] = false
		}
		if props, ok := m["properties"].(map[string]any); ok {
			for _, p := range props {
				andar(p)
			}
		}
		andar(m["items"])
	}
	andar(raiz)
	b, err := json.Marshal(raiz)
	if err != nil {
		return json.RawMessage(esquema)
	}
	return b
}

// chamarAPI e a reserva: Messages API por HTTP, sem ferramentas, mesma saida em JSON.
func chamarAPI(chave, modelo, sistema, entrada, esquema string, limite time.Duration) (json.RawMessage, error) {
	bruto, status, corpo, err := postarAPI(chave, modelo, sistema, entrada, esquema, true, limite)
	if err != nil && status == http.StatusBadRequest && strings.Contains(corpo, "output_config") {
		// Conta ou versao sem saida estruturada: pede o JSON pelas instrucoes.
		instrucao := "\n\nResponda SOMENTE com um objeto JSON válido, sem texto antes ou depois, seguindo este esquema: " + esquema + "\n"
		bruto, _, _, err = postarAPI(chave, modelo, sistema+instrucao, entrada, esquema, false, limite)
	}
	return bruto, err
}

func postarAPI(chave, modelo, sistema, entrada, esquema string, estruturado bool, limite time.Duration) (json.RawMessage, int, string, error) {
	pedido := map[string]any{
		"model":      modelo,
		"max_tokens": 16000,
		"system":     sistema,
		"messages":   []map[string]any{{"role": "user", "content": entrada}},
	}
	if estruturado {
		pedido["output_config"] = map[string]any{
			"format": map[string]any{"type": "json_schema", "schema": comAdicionaisFalsos(esquema)},
		}
	}
	corpo, err := json.Marshal(pedido)
	if err != nil {
		return nil, 0, "", err
	}
	ctx, cancelar := context.WithTimeout(context.Background(), limite)
	defer cancelar()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.anthropic.com/v1/messages", bytes.NewReader(corpo))
	if err != nil {
		return nil, 0, "", err
	}
	req.Header.Set("content-type", "application/json")
	req.Header.Set("x-api-key", chave)
	req.Header.Set("anthropic-version", "2023-06-01")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, 0, "", fmt.Errorf("sem resposta da API: %v", err)
	}
	defer resp.Body.Close()
	dados, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, resp.StatusCode, "", err
	}

	var r struct {
		StopReason string `json:"stop_reason"`
		Content    []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
		Error *struct {
			Type    string `json:"type"`
			Message string `json:"message"`
		} `json:"error"`
	}
	_ = json.Unmarshal(dados, &r)
	if resp.StatusCode != http.StatusOK {
		msg := cortar(string(dados), 300)
		if r.Error != nil {
			msg = r.Error.Type + ": " + r.Error.Message
		}
		return nil, resp.StatusCode, string(dados), fmt.Errorf("a API respondeu %d: %s", resp.StatusCode, cortar(msg, 300))
	}
	if r.StopReason == "refusal" {
		return nil, resp.StatusCode, "", errors.New("o modelo recusou responder esta mensagem")
	}
	if r.StopReason == "max_tokens" {
		return nil, resp.StatusCode, "", errors.New("a resposta da API foi cortada")
	}
	for _, bloco := range r.Content {
		if bloco.Type != "text" || strings.TrimSpace(bloco.Text) == "" {
			continue
		}
		if j := extrairJSON(bloco.Text); json.Valid([]byte(j)) {
			return json.RawMessage(j), resp.StatusCode, "", nil
		}
	}
	return nil, resp.StatusCode, "", errors.New("a API não devolveu o JSON esperado")
}

// modoTeste funciona sem claude e sem chave: resposta fixa.
func modoTeste(texto string) Saida {
	t := strings.ToLower(texto)
	if strings.Contains(t, "atendente") || strings.Contains(t, "humano") {
		return Saida{
			Resposta:    "Certo! Vou chamar uma pessoa da equipe para continuar seu atendimento por aqui.",
			Transbordo:  true,
			Motivo:      "pediu atendimento humano (modo teste)",
			Lead:        LeadIA{Classificacao: "indefinido", Etapa: "modo teste"},
			Agendamento: AgendamentoIA{Acao: "nenhuma"},
		}
	}
	return Saida{
		Resposta:    "Olá! Recebi sua mensagem. Este atendente está em modo teste. Se quiser falar com uma pessoa, escreva atendente.",
		Lead:        LeadIA{Classificacao: "indefinido", Etapa: "modo teste"},
		Agendamento: AgendamentoIA{Acao: "nenhuma"},
	}
}

func leadParaIA(l *Lead) LeadIA {
	if l == nil {
		return LeadIA{Classificacao: "indefinido"}
	}
	return LeadIA{
		Nome: l.Nome, Necessidade: l.Necessidade, Orcamento: l.Orcamento,
		Decisor: l.Decisor, Prazo: l.Prazo, Pontuacao: numFlex(l.Pontuacao),
		Classificacao: ouPadrao(l.Classificacao, "indefinido"), Etapa: l.Etapa, Resumo: l.Resumo,
	}
}

// montarEntrada cria o texto que vai pelo stdin: ultimas 12 mensagens, a nova no fim
// e a linha "Lead ate agora" com o que ja sabemos.
func montarEntrada(anteriores []Msg, novas []string, l *Lead) string {
	// As linhas de historico interno (quem assumiu, quem devolveu) nao vao para a IA.
	conversa := make([]Msg, 0, len(anteriores))
	for _, m := range anteriores {
		if m.De != "sistema" {
			conversa = append(conversa, m)
		}
	}
	anteriores = conversa
	if len(anteriores) > 12 {
		anteriores = anteriores[len(anteriores)-12:]
	}
	var b strings.Builder
	for _, m := range anteriores {
		quem := "Cliente"
		switch m.De {
		case "atendente":
			quem = "Atendente"
		case "humano":
			quem = "Equipe"
		}
		b.WriteString(quem + ": " + umaLinha(m.textoPlano()) + "\n")
	}
	limpas := make([]string, 0, len(novas))
	for _, n := range novas {
		if t := umaLinha(n); t != "" {
			limpas = append(limpas, t)
		}
	}
	b.WriteString("Cliente: " + strings.Join(limpas, " / ") + "\n")

	j, _ := json.Marshal(leadParaIA(l))
	b.WriteString("Lead até agora: " + string(j) + "\n")
	if l != nil && l.Retomado {
		b.WriteString("Observação: uma pessoa da equipe já atendeu este cliente e devolveu a conversa para você. Não chame uma pessoa de novo só porque o lead é quente. Chame apenas se o cliente pedir ou se houver outro motivo das regras.\n")
	}
	return b.String()
}

var camposVazios = map[string]bool{
	"indefinido": true, "não informado": true, "nao informado": true, "não informou": true,
	"nao informou": true, "desconhecido": true, "n/a": true, "-": true, "?": true,
	"não sei": true, "nao sei": true, "ainda não informado": true, "ainda nao informado": true,
	"não mencionado": true, "nao mencionado": true, "null": true, "nenhum": true,
}

func limparCampo(v string) string {
	v = strings.TrimSpace(v)
	if camposVazios[strings.ToLower(strings.TrimRight(v, "."))] {
		return ""
	}
	return cortar(v, 600)
}

func classePorPontos(p int) string {
	switch {
	case p >= 70:
		return "quente"
	case p >= 40:
		return "morno"
	default:
		return "frio"
	}
}

// aplicarSaida atualiza o lead sem apagar o que ja se sabia: campo vazio nao sobrescreve.
func aplicarSaida(l *Lead, s Saida) {
	por := func(destino *string, valor string) {
		if v := limparCampo(valor); v != "" {
			*destino = v
		}
	}
	por(&l.Nome, s.Lead.Nome)
	por(&l.Necessidade, s.Lead.Necessidade)
	por(&l.Orcamento, s.Lead.Orcamento)
	por(&l.Decisor, s.Lead.Decisor)
	por(&l.Prazo, s.Lead.Prazo)
	por(&l.Etapa, s.Lead.Etapa)
	por(&l.Resumo, s.Lead.Resumo)

	pontos := limitar(int(s.Lead.Pontuacao), 0, 100)
	classe := strings.ToLower(strings.TrimSpace(s.Lead.Classificacao))
	switch classe {
	case "quente", "morno", "frio":
		// Quente vale se o modelo disse quente ou se os pontos chegaram a 70.
		// Nos outros casos a classe segue a regra dos pontos.
		l.Pontuacao = pontos
		if classe == "quente" {
			l.Classificacao = "quente"
		} else {
			l.Classificacao = classePorPontos(pontos)
		}
	default:
		if l.Classificacao == "" {
			l.Classificacao = "indefinido"
		}
		if l.Classificacao == "indefinido" && pontos > 0 {
			l.Pontuacao = pontos
		}
	}
}
