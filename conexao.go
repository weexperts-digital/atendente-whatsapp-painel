package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"time"
)

// Conexao da IA: o login do Claude feito por um botao do painel.
//
// O programa nunca ve a senha de ninguem. Ele so inicia o comando oficial
// "claude auth login", que abre o navegador na pagina do Claude. A pessoa entra e
// autoriza la, e o proprio claude guarda o acesso no cofre do sistema. O comando e
// fixo: nada do que vem da pagina vira argumento.

const (
	prazoLogin      = 10 * time.Minute // tempo para a pessoa autorizar no navegador
	validadeConexao = 20 * time.Second // de quanto em quanto tempo o painel confere de novo
	vigiaConexao    = 10 * time.Minute // conferencia de rotina, em segundo plano
)

// estadoIA diz se o claude deste computador esta pronto para responder.
type estadoIA struct {
	Instalado bool   `json:"instalado"`
	Conectado bool   `json:"conectado"`
	Conta     string `json:"conta,omitempty"`
	Plano     string `json:"plano,omitempty"`
	sabe      bool   // o claude respondeu a pergunta sobre o login
}

// situacaoIA e o resumo que o painel inteiro usa: quem responde e se esta pronto.
type situacaoIA struct {
	Provedor  string `json:"provedor"`
	Instalado bool   `json:"instalado"`
	Conectado bool   `json:"conectado"`
	Motivo    string `json:"motivo"` // por que nao esta pronto: claude_login, openai_chave, openai_credito, openai_modelo
}

// situacaoDaIA olha o provedor em uso. Com o ChatGPT, "conectado" quer dizer chave
// guardada e sem falha de chave, credito ou modelo na ultima resposta.
func (a *App) situacaoDaIA(claude estadoIA) situacaoIA {
	falha, _ := a.cerebro.ultimos()
	motivo, _ := motivoDaFalha(falha)
	if chave, _ := a.cerebro.viaOpenAI(); chave != "" {
		return situacaoIA{Provedor: provedorOpenAI, Instalado: true, Conectado: motivo == "", Motivo: motivo}
	}
	e := a.estadoDaIA(claude)
	s := situacaoIA{Provedor: provedorClaude, Instalado: e.Instalado, Conectado: e.Conectado}
	if e.Instalado && !e.Conectado {
		s.Motivo = "claude_login"
	}
	return s
}

type conexaoIA struct {
	conferindo sync.Mutex // uma conferencia por vez

	mu       sync.Mutex
	estado   estadoIA
	visto    time.Time
	cmd      *exec.Cmd // login em andamento
	entrada  io.WriteCloser
	endereco string // pagina de login de reserva, para quando o navegador nao abre sozinho
	erro     string
	desistiu bool
	quem     *Usuario // quem clicou em Conectar
	parar    context.CancelFunc
}

// conferirClaude pergunta ao claude se ele esta logado. Nao gasta nada da conta.
func conferirClaude() estadoIA {
	bin := acharClaude()
	if bin == "" {
		return estadoIA{}
	}
	e := estadoIA{Instalado: true}
	ctx, cancelar := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancelar()
	cmd := exec.CommandContext(ctx, bin, "auth", "status")
	cmd.Env = ambienteFiltrado()
	cmd.Dir = os.TempDir()
	saida, _ := cmd.Output() // deslogado, o claude sai com erro mas ainda escreve a resposta
	var r struct {
		Logado *bool  `json:"loggedIn"`
		Email  string `json:"email"`
		Plano  string `json:"subscriptionType"`
	}
	if json.Unmarshal(bytes.TrimSpace(saida), &r) == nil && r.Logado != nil {
		e.sabe = true
		e.Conectado = *r.Logado
		e.Conta = cortar(r.Email, 120)
		e.Plano = cortar(r.Plano, 40)
	}
	return e
}

// guardado devolve o ultimo estado conferido, sem conferir de novo. O segundo valor e
// falso enquanto a primeira conferencia nao terminou.
func (c *conexaoIA) guardado() (estadoIA, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.estado, !c.visto.IsZero()
}

// atual devolve o ultimo estado conferido e confere de novo quando ele ficou velho.
func (c *conexaoIA) atual(forcar bool) estadoIA {
	c.mu.Lock()
	velho := time.Since(c.visto) > validadeConexao
	emLogin := c.cmd != nil
	e := c.estado
	c.mu.Unlock()
	if emLogin || (!forcar && !velho) {
		return e
	}
	c.conferindo.Lock()
	defer c.conferindo.Unlock()
	c.mu.Lock()
	if !forcar && time.Since(c.visto) <= validadeConexao { // outro pedido acabou de conferir
		e = c.estado
		c.mu.Unlock()
		return e
	}
	c.mu.Unlock()
	e = conferirClaude()
	c.mu.Lock()
	c.estado, c.visto = e, time.Now()
	c.mu.Unlock()
	return e
}

// estadoDaIA junta o que o claude diz sobre o login com o que as ultimas respostas
// mostraram. Uma falha de login vale mais que o "logado" do claude, e uma resposta boa
// vale mais que o "deslogado".
func (a *App) estadoDaIA(e estadoIA) estadoIA {
	a.conexao.mu.Lock()
	visto := a.conexao.visto
	a.conexao.mu.Unlock()
	falha, acerto := a.cerebro.ultimos()
	switch {
	case avisoDoCerebro(falha) != "":
		e.Conectado = false
	case !e.sabe && e.Instalado:
		e.Conectado = true // claude antigo, sem "auth status": nao acusa sem prova
	case acerto.After(visto):
		e.Conectado = true
	}
	if os.Getenv("ANTHROPIC_API_KEY") != "" {
		e.Conectado = true // a reserva pela API responde mesmo sem o login
	}
	return e
}

// vigiarIA confere o login ao ligar e depois de tempos em tempos, para o painel avisar
// antes de um cliente ficar sem resposta.
func (a *App) vigiarIA() {
	for {
		a.conexao.atual(true)
		time.Sleep(vigiaConexao)
	}
}

var reEnderecoLogin = regexp.MustCompile(`visit:\s*(https://\S+)`)

// enderecoSeguro so aceita paginas do proprio Claude.
func enderecoSeguro(bruto string) string {
	u, err := url.Parse(bruto)
	if err != nil || u.Scheme != "https" || u.User != nil {
		return ""
	}
	host := strings.ToLower(u.Hostname())
	for _, dominio := range []string{"claude.com", "claude.ai", "anthropic.com"} {
		if host == dominio || strings.HasSuffix(host, "."+dominio) {
			return u.String()
		}
	}
	return ""
}

var errSemClaude = errors.New("o Claude Code não está instalado neste computador")

// iniciarLogin comeca o login oficial do claude. Se ja houver um em andamento, nao faz nada.
func (a *App) iniciarLogin() error {
	bin := acharClaude()
	if bin == "" {
		return errSemClaude
	}
	c := &a.conexao
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.cmd != nil {
		return nil
	}
	pasta, err := os.MkdirTemp("", "atendente-login-")
	if err != nil {
		return err
	}
	ctx, parar := context.WithTimeout(context.Background(), prazoLogin)
	cmd := exec.CommandContext(ctx, bin, "auth", "login", "--claudeai")
	cmd.Dir = pasta
	cmd.Env = ambienteFiltrado()
	cmd.WaitDelay = 3 * time.Second
	entrada, err := cmd.StdinPipe()
	if err == nil {
		var saida io.ReadCloser
		if saida, err = cmd.StdoutPipe(); err == nil {
			cmd.Stderr = cmd.Stdout
			if err = cmd.Start(); err == nil {
				c.cmd, c.entrada, c.parar = cmd, entrada, parar
				c.endereco, c.erro, c.desistiu = "", "", false
				go a.acompanharLogin(cmd, saida, pasta)
				return nil
			}
		}
	}
	parar()
	os.RemoveAll(pasta)
	return err
}

// acompanharLogin le o que o claude escreve (so para achar a pagina de reserva), espera
// ele terminar e confere se o login deu certo.
func (a *App) acompanharLogin(cmd *exec.Cmd, saida io.Reader, pasta string) {
	c := &a.conexao
	var lido []byte
	pedaco := make([]byte, 2048)
	for {
		n, err := saida.Read(pedaco)
		if n > 0 && len(lido) < 32<<10 {
			lido = append(lido, pedaco[:n]...)
			if m := reEnderecoLogin.FindSubmatch(lido); m != nil {
				if endereco := enderecoSeguro(string(m[1])); endereco != "" {
					c.mu.Lock()
					c.endereco = endereco
					c.mu.Unlock()
				}
			}
		}
		if err != nil {
			break
		}
	}
	_ = cmd.Wait()

	novo := conferirClaude()
	c.mu.Lock()
	c.parar()
	c.cmd, c.entrada, c.parar, c.endereco = nil, nil, nil, ""
	c.estado, c.visto = novo, time.Now()
	desistiu, quem := c.desistiu, c.quem
	if !novo.Conectado && !desistiu {
		c.erro = "O login não foi concluído. Clique em Conectar e tente de novo."
	}
	c.mu.Unlock()
	os.RemoveAll(pasta)

	if novo.Conectado {
		a.cerebro.esquecerFalhaDeLogin()
		a.registrar(quem, "ia_conectada", novo.Conta)
		log.Printf("IA: login do Claude concluído pelo painel")
	} else if !desistiu {
		log.Printf("IA: o login do Claude pelo painel não foi concluído")
	}
}

// visaoDaConexao e o que a janela "Conectar a IA" mostra.
func (a *App) visaoDaConexao(forcar bool) map[string]any {
	base := a.conexao.atual(forcar)
	e := a.estadoDaIA(base)
	s := a.situacaoDaIA(base)
	openai := a.visaoOpenAI()
	c := &a.conexao
	c.mu.Lock()
	defer c.mu.Unlock()
	return map[string]any{
		"provedor":         s.Provedor,
		"conectado":        s.Conectado, // do provedor em uso
		"motivo":           s.Motivo,
		"openai":           openai,
		"instalado":        e.Instalado, // daqui para baixo, o login do Claude
		"claude_conectado": e.Conectado,
		"conta":            e.Conta,
		"plano":            e.Plano,
		"em_andamento":     c.cmd != nil,
		"endereco":         c.endereco,
		"erro":             c.erro,
	}
}

func (a *App) rotaConexao(w http.ResponseWriter, r *http.Request) {
	responderJSON(w, http.StatusOK, a.visaoDaConexao(r.URL.Query().Get("agora") == "1"))
}

func (a *App) rotaConectar(w http.ResponseWriter, r *http.Request) {
	if !lerPedido(w, r, &struct{}{}) {
		return
	}
	if err := a.iniciarLogin(); err != nil {
		if errors.Is(err, errSemClaude) {
			responderErro(w, http.StatusServiceUnavailable, "O Claude Code não está instalado neste computador. Sem ele o assistente não consegue pensar.")
			return
		}
		log.Printf("IA: não consegui iniciar o login do Claude: %v", err)
		responderErro(w, http.StatusInternalServerError, "Não consegui abrir o login do Claude. Tente de novo.")
		return
	}
	a.conexao.mu.Lock()
	a.conexao.quem = usuarioDe(r)
	a.conexao.mu.Unlock()
	a.registrar(usuarioDe(r), "ia_login_iniciado", "")
	responderJSON(w, http.StatusOK, a.visaoDaConexao(false))
}

// rotaCodigoLogin entrega ao claude o codigo que a pagina de reserva mostra. O codigo
// vale uma vez so e nao e guardado nem escrito no log.
func (a *App) rotaCodigoLogin(w http.ResponseWriter, r *http.Request) {
	var p struct {
		Codigo string `json:"codigo"`
	}
	if !lerPedido(w, r, &p) {
		return
	}
	codigo := strings.TrimSpace(p.Codigo)
	if codigo == "" || len(codigo) > 1024 || strings.ContainsAny(codigo, "\r\n\x00 ") {
		responderErro(w, http.StatusBadRequest, "Cole o código do jeito que a página do Claude mostrou, sem espaços.")
		return
	}
	c := &a.conexao
	c.mu.Lock()
	entrada := c.entrada
	c.mu.Unlock()
	if entrada == nil {
		responderErro(w, http.StatusConflict, "O login não está em andamento. Clique em Conectar de novo.")
		return
	}
	if _, err := io.WriteString(entrada, codigo+"\n"); err != nil {
		responderErro(w, http.StatusConflict, "O login já terminou. Clique em Conectar de novo.")
		return
	}
	responderOK(w, nil)
}

func (a *App) rotaCancelarLogin(w http.ResponseWriter, r *http.Request) {
	if !lerPedido(w, r, &struct{}{}) {
		return
	}
	c := &a.conexao
	c.mu.Lock()
	if c.cmd != nil && c.parar != nil {
		c.desistiu = true
		c.parar() // encerra o comando; acompanharLogin faz a limpeza
	}
	c.erro = ""
	c.mu.Unlock()
	responderOK(w, nil)
}
