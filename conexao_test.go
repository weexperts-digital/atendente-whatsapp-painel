package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// claudeDeMentira imita so o que o painel usa do claude: "auth status" e "auth login".
// O login le um codigo da entrada. Com o codigo certo, passa a responder "logado".
const claudeDeMentira = `#!/bin/sh
if [ "$1" = "auth" ] && [ "$2" = "status" ]; then
  if [ -f "$FAKE_ESTADO" ]; then
    echo '{"loggedIn": true, "authMethod": "claude.ai", "email": "teste@exemplo.com", "subscriptionType": "max"}'
    exit 0
  fi
  echo '{"loggedIn": false, "authMethod": "none"}'
  exit 1
fi
if [ "$1" = "auth" ] && [ "$2" = "login" ]; then
  echo "Opening browser to sign in…"
  echo "If the browser didn't open, visit: https://claude.com/cai/oauth/authorize?code=true&state=abc"
  printf "Paste code here if prompted > "
  read codigo
  if [ "$codigo" = "codigo-certo" ]; then
    : > "$FAKE_ESTADO"
    echo "Login successful"
    exit 0
  fi
  echo "Login failed"
  exit 1
fi
exit 2
`

func comClaudeDeMentira(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("o claude de mentira é um script de shell")
	}
	pasta := t.TempDir()
	bin := filepath.Join(pasta, "claude")
	if err := os.WriteFile(bin, []byte(claudeDeMentira), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CLAUDE_BIN", bin)
	t.Setenv("FAKE_ESTADO", filepath.Join(pasta, "logado"))
	t.Setenv("ANTHROPIC_API_KEY", "")
}

func esperar(t *testing.T, oQue string, pronto func() bool) {
	t.Helper()
	for fim := time.Now().Add(10 * time.Second); time.Now().Before(fim); time.Sleep(20 * time.Millisecond) {
		if pronto() {
			return
		}
	}
	t.Fatalf("esperei 10 segundos e não aconteceu: %s", oQue)
}

func postar(rota http.HandlerFunc, corpo string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(corpo))
	r.Header.Set("Content-Type", "application/json")
	rota(w, r)
	return w
}

func TestLoginDoClaudePeloPainel(t *testing.T) {
	comClaudeDeMentira(t)
	a := novoApp()

	v := a.visaoDaConexao(true)
	if v["instalado"] != true || v["conectado"] != false || v["em_andamento"] != false {
		t.Fatalf("antes do login devia estar instalado e desconectado: %v", v)
	}
	if w := postar(a.rotaCodigoLogin, `{"codigo":"abc"}`); w.Code != http.StatusConflict {
		t.Errorf("código sem login em andamento devia dar 409, deu %d", w.Code)
	}

	if w := postar(a.rotaConectar, `{}`); w.Code != http.StatusOK {
		t.Fatalf("conectar devia responder 200, deu %d: %s", w.Code, w.Body.String())
	}
	esperar(t, "a página de reserva aparecer", func() bool { return a.visaoDaConexao(false)["endereco"] != "" })
	v = a.visaoDaConexao(false)
	if v["em_andamento"] != true || !strings.HasPrefix(v["endereco"].(string), "https://claude.com/") {
		t.Fatalf("login em andamento devia mostrar a página do Claude: %v", v)
	}
	if w := postar(a.rotaConectar, `{}`); w.Code != http.StatusOK {
		t.Errorf("clicar em conectar duas vezes não pode dar erro: %d", w.Code)
	}

	for _, ruim := range []string{`{"codigo":""}`, `{"codigo":"tem espaco"}`, `{"codigo":"linha\nnova"}`} {
		if w := postar(a.rotaCodigoLogin, ruim); w.Code != http.StatusBadRequest {
			t.Errorf("código %s devia ser recusado com 400, deu %d", ruim, w.Code)
		}
	}
	if w := postar(a.rotaCodigoLogin, `{"codigo":"codigo-certo"}`); w.Code != http.StatusOK {
		t.Fatalf("código certo devia ser aceito: %d %s", w.Code, w.Body.String())
	}
	esperar(t, "o login terminar", func() bool { return a.visaoDaConexao(false)["em_andamento"] == false })
	v = a.visaoDaConexao(false)
	if v["conectado"] != true || v["conta"] != "teste@exemplo.com" || v["plano"] != "max" || v["erro"] != "" || v["endereco"] != "" {
		t.Errorf("depois do login devia estar conectado, com a conta e sem erro: %v", v)
	}
}

func TestLoginDoClaudeQueNaoTermina(t *testing.T) {
	comClaudeDeMentira(t)
	a := novoApp()

	// Código errado: o claude sai com erro e o painel explica.
	if err := a.iniciarLogin(); err != nil {
		t.Fatal(err)
	}
	esperar(t, "o login começar", func() bool { return a.visaoDaConexao(false)["endereco"] != "" })
	postar(a.rotaCodigoLogin, `{"codigo":"codigo-errado"}`)
	esperar(t, "o login terminar", func() bool { return a.visaoDaConexao(false)["em_andamento"] == false })
	v := a.visaoDaConexao(false)
	if v["conectado"] != false || !strings.Contains(v["erro"].(string), "não foi concluído") {
		t.Errorf("login que falhou devia avisar e seguir desconectado: %v", v)
	}

	// Cancelar: encerra o comando e não deixa aviso de erro.
	if err := a.iniciarLogin(); err != nil {
		t.Fatal(err)
	}
	esperar(t, "o login começar de novo", func() bool { return a.visaoDaConexao(false)["em_andamento"] == true })
	if w := postar(a.rotaCancelarLogin, `{}`); w.Code != http.StatusOK {
		t.Fatalf("cancelar devia responder 200, deu %d", w.Code)
	}
	esperar(t, "o login cancelado sair", func() bool { return a.visaoDaConexao(false)["em_andamento"] == false })
	if v := a.visaoDaConexao(false); v["erro"] != "" || v["conectado"] != false {
		t.Errorf("cancelar não devia deixar erro: %v", v)
	}
}

func TestEstadoDaIAOuveAsRespostas(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "")
	a := novoApp()
	logado := estadoIA{Instalado: true, Conectado: true, sabe: true}
	deslogado := estadoIA{Instalado: true, Conectado: false, sabe: true}

	if !a.estadoDaIA(logado).Conectado || a.estadoDaIA(deslogado).Conectado {
		t.Error("sem notícia das respostas, vale o que o claude disse")
	}
	if !a.estadoDaIA(estadoIA{Instalado: true}).Conectado {
		t.Error("claude antigo, que não informa o login, não pode ser acusado de desconectado")
	}

	a.cerebro.ultimoErro = "o claude avisou um erro: Failed to authenticate: OAuth session expired and could not be refreshed"
	if a.estadoDaIA(logado).Conectado {
		t.Error("falha de login numa resposta vale mais que o logado do claude")
	}
	a.cerebro.esquecerFalhaDeLogin()
	if a.cerebro.ultimoErro != "" || !a.estadoDaIA(logado).Conectado {
		t.Error("depois de entrar de novo, a falha de login tem de sumir")
	}

	a.cerebro.ultimoAcerto = time.Now()
	if !a.estadoDaIA(deslogado).Conectado {
		t.Error("resposta boa depois da conferência prova que a IA está conectada")
	}

	t.Setenv("ANTHROPIC_API_KEY", "chave-de-teste")
	a.cerebro.ultimoAcerto = time.Time{}
	if !a.estadoDaIA(deslogado).Conectado {
		t.Error("com a reserva pela API, a falta do login não derruba a IA")
	}
}

func TestEnderecoSeguroSoAceitaPaginasDoClaude(t *testing.T) {
	bons := []string{
		"https://claude.com/cai/oauth/authorize?code=true",
		"https://claude.ai/oauth/authorize?x=1",
		"https://platform.claude.com/oauth/code/callback",
		"https://console.anthropic.com/oauth/authorize",
	}
	for _, e := range bons {
		if enderecoSeguro(e) == "" {
			t.Errorf("devia aceitar %s", e)
		}
	}
	ruins := []string{
		"http://claude.com/cai/oauth/authorize",
		"https://claude.com.golpe.example/login",
		"https://golpe.example/claude.com",
		"https://usuario:senha@claude.com/",
		"javascript:alert(1)",
		"",
	}
	for _, e := range ruins {
		if enderecoSeguro(e) != "" {
			t.Errorf("não devia aceitar %q", e)
		}
	}
}
