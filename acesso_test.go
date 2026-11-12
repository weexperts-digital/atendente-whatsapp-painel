package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestSenhaGuardadaComHash(t *testing.T) {
	const senha = "uma frase só minha 42"
	h1, err := gerarHash(senha)
	if err != nil {
		t.Fatal(err)
	}
	h2, _ := gerarHash(senha)
	if h1 == h2 {
		t.Errorf("a mesma senha gerou o mesmo código duas vezes: falta o sal")
	}
	if strings.Contains(h1, senha) || !strings.HasPrefix(h1, "$argon2id$") {
		t.Errorf("o código guardado não parece um hash Argon2id: %q", h1)
	}
	if !conferirHash(senha, h1) || !conferirHash(senha, h2) {
		t.Errorf("a senha certa não conferiu")
	}
	for _, errada := range []string{"", "outra senha qualquer", senha + " ", strings.ToUpper(senha)} {
		if conferirHash(errada, h1) {
			t.Errorf("a senha errada %q conferiu", errada)
		}
	}
	for _, quebrado := range []string{"", "texto puro", "$argon2id$v=19$m=19456,t=2,p=1$só-isso", "$argon2i$v=19$m=19456,t=2,p=1$YWJjZGVmZ2g$YWJjZGVmZ2hpamtsbW5vcA", "$argon2id$v=19$m=99999999,t=2,p=1$YWJjZGVmZ2g$YWJjZGVmZ2hpamtsbW5vcA"} {
		if conferirHash(senha, quebrado) {
			t.Errorf("um hash quebrado conferiu: %q", quebrado)
		}
	}
}

func TestSenhaAceita(t *testing.T) {
	casos := []struct {
		senha, email, nome string
		aceita             bool
	}{
		{"curta", "ana@loja.com", "Ana", false},
		{"12345678", "ana@loja.com", "Ana", false},
		{"Senha123", "ana@loja.com", "Ana", false},
		{"aaaaaaaaaa", "ana@loja.com", "Ana", false},
		{"anapaula2026", "anapaula@loja.com", "Ana", false},
		{"ana@loja.com", "ana@loja.com", "Ana", false},
		{"Ana Paula", "x@loja.com", "ana paula", false},
		{strings.Repeat("x1", 70), "ana@loja.com", "Ana", false},
		{"cavalo-bateria-grampo", "ana@loja.com", "Ana", true},
		{"Tr0co!2026", "ana@loja.com", "Ana", true},
		{"çãõéíóú123", "ana@loja.com", "Ana", true},
	}
	for _, c := range casos {
		motivo := senhaAceita(c.senha, c.email, c.nome)
		if (motivo == "") != c.aceita {
			t.Errorf("senhaAceita(%q) = %q, esperava aceita=%v", c.senha, motivo, c.aceita)
		}
	}
}

func TestEmailLimpo(t *testing.T) {
	casos := map[string]string{
		" Ana@Loja.COM ":   "ana@loja.com",
		"ana.p+x@loja.co":  "ana.p+x@loja.co",
		"":                 "",
		"ana":              "",
		"ana@":             "",
		"@loja.com":        "",
		"ana@loja":         "",
		"ana@loja.c":       "",
		"ana@@loja.com":    "",
		"ana maria@x.com":  "",
		"ana@loja.com>":    "",
		"a@b.com, c@d.com": "",
	}
	for entrada, esperado := range casos {
		if got := emailLimpo(entrada); got != esperado {
			t.Errorf("emailLimpo(%q) = %q, esperava %q", entrada, got, esperado)
		}
	}
}

func TestNivelDaRota(t *testing.T) {
	casos := []struct {
		metodo, caminho string
		nivel           int
	}{
		{"GET", "/", nivelLivre},
		{"GET", "/app.js", nivelLivre},
		{"GET", "/vendor/motion.js", nivelLivre},
		{"GET", "/api/sessao", nivelLivre},
		{"POST", "/api/entrar", nivelLivre},
		{"POST", "/api/primeiro-acesso", nivelLivre},
		{"GET", "/api/estado", nivelLogado},
		{"GET", "/api/contatos", nivelLogado},
		{"GET", "/contatos.csv", nivelLogado},
		{"GET", "/leads.csv", nivelLogado},
		{"GET", "/api/midia", nivelLogado},
		{"POST", "/api/responder", nivelLogado},
		{"POST", "/api/senha", nivelLogado},
		{"GET", "/api/rota-que-nao-existe", nivelLogado},
		{"GET", "/api/config", nivelLogado},
		{"POST", "/api/config", nivelAdmin},
		{"GET", "/api/agenda", nivelLogado},
		{"POST", "/api/agenda", nivelAdmin},
		{"POST", "/api/agenda/marcar", nivelLogado},
		{"POST", "/api/liga", nivelAdmin},
		{"GET", "/qr.png", nivelAdmin},
		{"POST", "/api/lead/excluir", nivelAdmin},
		{"GET", "/api/textos", nivelAdmin},
		{"POST", "/api/conhecimento/enviar", nivelAdmin},
		{"GET", "/api/ia/conexao", nivelAdmin},
		{"POST", "/api/ia/conectar", nivelAdmin},
		{"POST", "/api/ia/conectar/codigo", nivelAdmin},
		{"POST", "/api/ia/conectar/cancelar", nivelAdmin},
		{"GET", "/api/usuarios", nivelSuper},
		{"POST", "/api/usuarios/alterar", nivelSuper},
		{"GET", "/api/registro", nivelSuper},
	}
	for _, c := range casos {
		if got := nivelDaRota(c.metodo, c.caminho); got != c.nivel {
			t.Errorf("nivelDaRota(%s %s) = %d, esperava %d", c.metodo, c.caminho, got, c.nivel)
		}
	}
}

func TestPortariaSeguraTentativas(t *testing.T) {
	p := novaPortaria()
	agora := time.Now()
	for i := 0; i < maxFalhas-1; i++ {
		p.falhou("ana@loja.com", agora)
	}
	if p.espera("ana@loja.com", agora) != 0 {
		t.Fatalf("bloqueou antes de %d erros", maxFalhas)
	}
	p.falhou("ana@loja.com", agora)
	if falta := p.espera("ana@loja.com", agora); falta <= 0 || falta > janelaFalhas {
		t.Fatalf("depois de %d erros devia segurar, espera = %v", maxFalhas, falta)
	}
	if p.espera("bia@loja.com", agora) != 0 {
		t.Errorf("o bloqueio de um e-mail pegou outro")
	}
	if p.espera("ana@loja.com", agora.Add(janelaFalhas+time.Second)) != 0 {
		t.Errorf("o bloqueio não acabou depois do prazo")
	}
	p.acertou("ana@loja.com")
	if p.espera("ana@loja.com", agora) != 0 {
		t.Errorf("entrar com sucesso devia zerar as tentativas")
	}
}

func TestSessaoValeEExpira(t *testing.T) {
	b := bancoDeTeste(t)
	hash, _ := gerarHash("cavalo-bateria-grampo")
	u, err := b.criarUsuario("Ana", "ana@loja.com", papelSuper, hash, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := b.criarUsuario("Outra", "outra@loja.com", papelSuper, hash, false); err == nil {
		t.Errorf("deixou criar um segundo super administrador")
	}
	if _, err := b.criarUsuario("Ana 2", "ana@loja.com", papelAtendente, hash, true); err != errEmailEmUso {
		t.Errorf("e-mail repetido devia ser recusado, veio %v", err)
	}
	agora := time.Now()
	codigo, err := b.criarSessao(u.ID, "teste", agora)
	if err != nil || len(codigo) != 64 {
		t.Fatalf("criando sessão: %v (%d)", err, len(codigo))
	}
	if n := contar(t, b, `SELECT COUNT(*) FROM sessoes WHERE chave = ?`, codigo); n != 0 {
		t.Errorf("o código da sessão foi gravado puro no banco")
	}
	if got := b.usuarioDaSessao(codigo, agora.Add(time.Hour)); got == nil || got.ID != u.ID {
		t.Fatalf("a sessão recém-criada não valeu")
	}
	if b.usuarioDaSessao(strings.Repeat("a", 64), agora) != nil || b.usuarioDaSessao("", agora) != nil {
		t.Errorf("um código inventado valeu como sessão")
	}
	if b.usuarioDaSessao(codigo, agora.Add(sessaoParada+time.Hour)) != nil {
		t.Errorf("a sessão parada há muito tempo continuou valendo")
	}
	if n := contar(t, b, `SELECT COUNT(*) FROM sessoes`); n != 0 {
		t.Errorf("a sessão vencida não foi apagada")
	}
	codigo, _ = b.criarSessao(u.ID, "teste", agora)
	_ = b.executar(`UPDATE usuarios SET ativo = 0 WHERE id = ?`, u.ID)
	if b.usuarioDaSessao(codigo, agora) != nil {
		t.Errorf("conta desativada continuou com sessão")
	}
}

// Passa pelo painel de verdade (com o filtro de acesso na frente): sem conta, cria
// o super administrador, cria um atendente e confere o que cada um alcança.
func TestPainelExigeLogin(t *testing.T) {
	pastaDeTeste(t)
	a := novoApp()
	a.banco = bancoDeTeste(t)
	a.cookie = "atendente_teste"
	srv := httptest.NewServer(a.protegido(a.rotas()))
	defer srv.Close()

	novoCliente := func() *http.Client {
		jar, _ := cookiejar.New(nil)
		return &http.Client{Jar: jar}
	}
	pedir := func(c *http.Client, metodo, caminho string, corpo any) (int, map[string]any) {
		t.Helper()
		var dados []byte
		if corpo != nil {
			dados, _ = json.Marshal(corpo)
		}
		req, _ := http.NewRequest(metodo, srv.URL+caminho, bytes.NewReader(dados))
		if corpo != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		resp, err := c.Do(req)
		if err != nil {
			t.Fatalf("%s %s: %v", metodo, caminho, err)
		}
		defer resp.Body.Close()
		var saida map[string]any
		_ = json.NewDecoder(resp.Body).Decode(&saida)
		return resp.StatusCode, saida
	}
	espera := func(nome string, codigo, esperado int) {
		t.Helper()
		if codigo != esperado {
			t.Errorf("%s: código %d, esperava %d", nome, codigo, esperado)
		}
	}

	anonimo := novoCliente()
	codigo, r := pedir(anonimo, "GET", "/api/sessao", nil)
	if codigo != 200 || r["precisa_criar"] != true {
		t.Fatalf("sem conta, a sessão devia pedir a criação: %d %v", codigo, r)
	}
	for _, rota := range []string{"/api/estado", "/api/config", "/api/contatos", "/api/usuarios", "/api/registro", "/leads.csv", "/contatos.csv", "/agenda.csv", "/qr.png", "/api/midia?id=x", "/api/foto?id=x", "/api/nao-existe"} {
		codigo, _ = pedir(anonimo, "GET", rota, nil)
		espera("sem login "+rota, codigo, 401)
	}
	codigo, _ = pedir(anonimo, "GET", "/", nil)
	espera("a página abre sem login", codigo, 200)

	const senhaDono, senhaProv, senhaBia = "cavalo-bateria-grampo", "provisoria-7391-x", "janela-verde-caneca"
	codigo, r = pedir(anonimo, "POST", "/api/primeiro-acesso", map[string]any{"nome": "Dona", "email": "dona@loja.com", "senha": senhaDono, "senha2": "diferente-da-outra"})
	if codigo != 400 || r["campo"] != "senha2" {
		t.Errorf("senhas diferentes: %d %v", codigo, r)
	}
	dono := novoCliente()
	codigo, r = pedir(dono, "POST", "/api/primeiro-acesso", map[string]any{"nome": "Dona", "email": "Dona@Loja.com", "senha": senhaDono, "senha2": senhaDono})
	if codigo != 200 {
		t.Fatalf("criando o super administrador: %d %v", codigo, r)
	}
	if u, _ := r["usuario"].(map[string]any); u["papel"] != papelSuper || u["email"] != "dona@loja.com" || u["senha"] != nil {
		t.Errorf("conta criada veio errada: %v", r["usuario"])
	}
	codigo, _ = pedir(anonimo, "POST", "/api/primeiro-acesso", map[string]any{"nome": "Intruso", "email": "x@loja.com", "senha": senhaDono, "senha2": senhaDono})
	espera("segundo super administrador", codigo, 409)
	codigo, _ = pedir(dono, "GET", "/api/estado", nil)
	espera("dono vê o painel", codigo, 200)
	codigo, _ = pedir(anonimo, "GET", "/api/estado", nil)
	espera("anônimo continua de fora", codigo, 401)

	codigo, r = pedir(dono, "POST", "/api/usuarios", map[string]any{"nome": "Bia", "email": "bia@loja.com", "papel": papelAtendente, "senha": senhaProv})
	if codigo != 200 {
		t.Fatalf("criando atendente: %d %v", codigo, r)
	}
	codigo, _ = pedir(dono, "POST", "/api/usuarios", map[string]any{"nome": "Outro", "email": "o@loja.com", "papel": papelSuper, "senha": senhaProv})
	espera("criar outro super administrador pela equipe", codigo, 400)

	bia := novoCliente()
	codigo, _ = pedir(bia, "POST", "/api/entrar", map[string]any{"email": "bia@loja.com", "senha": "senha-errada-aqui"})
	espera("senha errada", codigo, 401)
	codigo, _ = pedir(bia, "POST", "/api/entrar", map[string]any{"email": "bia@loja.com", "senha": senhaProv})
	espera("entra com a provisória", codigo, 200)
	codigo, r = pedir(bia, "GET", "/api/estado", nil)
	if codigo != 403 || r["trocar_senha"] != true {
		t.Errorf("antes de criar a senha, o painel devia ficar fechado: %d %v", codigo, r)
	}
	codigo, _ = pedir(bia, "POST", "/api/senha", map[string]any{"atual": senhaProv, "nova": senhaBia, "nova2": senhaBia})
	espera("cria a própria senha", codigo, 200)
	codigo, _ = pedir(bia, "GET", "/api/estado", nil)
	espera("atendente vê as conversas", codigo, 200)
	codigo, _ = pedir(bia, "GET", "/api/contatos", nil)
	espera("atendente vê os contatos", codigo, 200)
	for rota, corpo := range map[string]any{
		"/api/config":               map[string]any{"negocio": "Invasão"},
		"/api/liga":                 map[string]any{"ligar": false},
		"/api/whatsapp/desconectar": map[string]any{},
		"/api/lead/excluir":         map[string]any{"id": "x"},
		"/api/textos":               map[string]any{"arquivo": "cerebro", "texto": "x"},
		"/api/usuarios":             map[string]any{"nome": "Z", "email": "z@loja.com", "papel": papelAdmin, "senha": senhaBia},
	} {
		codigo, _ = pedir(bia, "POST", rota, corpo)
		espera("atendente barrado em "+rota, codigo, 403)
	}
	codigo, _ = pedir(bia, "GET", "/api/registro", nil)
	espera("atendente não vê o registro", codigo, 403)

	codigo, _ = pedir(bia, "POST", "/api/sair", map[string]any{})
	espera("sair", codigo, 200)
	codigo, _ = pedir(bia, "GET", "/api/estado", nil)
	espera("depois de sair", codigo, 401)

	// Senha errada repetida segura o e-mail, mesmo com a senha certa depois.
	for i := 0; i < maxFalhas; i++ {
		pedir(anonimo, "POST", "/api/entrar", map[string]any{"email": "bia@loja.com", "senha": "chute-numero-x"})
	}
	codigo, _ = pedir(anonimo, "POST", "/api/entrar", map[string]any{"email": "bia@loja.com", "senha": senhaBia})
	espera("bloqueio por tentativas", codigo, 429)
	codigo, _ = pedir(dono, "GET", "/api/estado", nil)
	espera("o bloqueio não derruba quem já entrou", codigo, 200)

	codigo, r = pedir(dono, "GET", "/api/registro?limite=100", nil)
	registros, _ := r["registros"].([]any)
	feitos := map[string]bool{}
	for _, x := range registros {
		if linha, ok := x.(map[string]any); ok {
			feitos[linha["acao"].(string)] = true
		}
	}
	for _, acao := range []string{"conta_criada", "usuario_criado", "entrou", "senha_errada", "senha_trocada", "saiu", "entrada_bloqueada"} {
		if !feitos[acao] {
			t.Errorf("o registro de acessos não anotou %q (código %d)", acao, codigo)
		}
	}
	bruto, _ := json.Marshal(r)
	for _, segredo := range []string{senhaDono, senhaProv, senhaBia} {
		if bytes.Contains(bruto, []byte(segredo)) {
			t.Errorf("uma senha apareceu no registro de acessos")
		}
	}
}
