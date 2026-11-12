package main

import (
	"context"
	crand "crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"golang.org/x/crypto/argon2"
)

// Quem entra no painel. A primeira conta criada e a do super administrador, que
// escolhe a propria senha. Ele cria as outras contas. Nao existe recuperacao por
// e-mail: quem perde a senha do super administrador usa "./atendente -esqueci-senha".
const (
	papelSuper     = "superadmin" // tudo, inclusive a equipe e o registro de acessos
	papelAdmin     = "admin"      // tudo, menos a equipe e o registro de acessos
	papelAtendente = "atendente"  // conversas, contatos, agenda e relatorio

	nivelLivre  = 0
	nivelLogado = 1
	nivelAdmin  = 2
	nivelSuper  = 3

	sessaoParada = 7 * 24 * time.Hour  // sem uso por esse tempo, a sessao acaba
	sessaoMaxima = 30 * 24 * time.Hour // depois disso, a pessoa entra de novo

	minSenha = 8
	maxSenha = 128

	maxFalhas       = 5 // senhas erradas seguidas para o mesmo e-mail
	maxFalhasGeral  = 40
	janelaFalhas    = 15 * time.Minute
	prazoRedefinir  = 30 * time.Minute
	esperaAposFalha = 400 * time.Millisecond
)

var nomesDePapel = map[string]string{
	papelSuper:     "Super administrador",
	papelAdmin:     "Administrador",
	papelAtendente: "Atendente",
}

func nivelDoPapel(papel string) int {
	switch papel {
	case papelSuper:
		return nivelSuper
	case papelAdmin:
		return nivelAdmin
	case papelAtendente:
		return nivelLogado
	}
	return nivelLivre
}

// nivelDaRota diz quem pode chamar cada endereco. O que nao esta na lista e e do
// painel (/api/...) ou planilha (.csv) exige estar logado: o padrao e fechar.
func nivelDaRota(metodo, caminho string) int {
	switch caminho {
	case "/api/sessao", "/api/entrar", "/api/sair", "/api/primeiro-acesso", "/api/redefinir-senha":
		return nivelLivre
	case "/api/usuarios", "/api/usuarios/alterar", "/api/usuarios/excluir", "/api/registro":
		return nivelSuper
	case "/api/liga", "/api/whatsapp/desconectar", "/qr.png", "/api/lead/excluir",
		"/api/textos", "/api/textos/restaurar",
		"/api/ia/negocio", "/api/ia/melhorar", "/api/ia/cerebro",
		"/api/ia/conexao", "/api/ia/conectar", "/api/ia/conectar/codigo", "/api/ia/conectar/cancelar",
		"/api/ia/openai", "/api/ia/openai/modelos", "/api/ia/openai/remover", "/api/ia/provedor",
		"/api/etiquetas/renomear", "/api/etiquetas/excluir", "/api/treinar", "/api/aprendizado",
		"/api/conhecimento/enviar", "/api/conhecimento/texto", "/api/conhecimento/alterar", "/api/conhecimento/excluir":
		return nivelAdmin
	case "/api/config", "/api/agenda":
		if metodo != http.MethodGet && metodo != http.MethodHead {
			return nivelAdmin
		}
		return nivelLogado
	}
	if strings.HasPrefix(caminho, "/api/") || strings.HasSuffix(caminho, ".csv") {
		return nivelLogado
	}
	// Arquivos do reconhecimento de voz: só para quem entrou (voz.go confere de novo).
	if strings.HasPrefix(caminho, "/voz/") {
		return nivelLogado
	}
	return nivelLivre // os arquivos da pagina (a tela de entrada precisa deles)
}

// Usuario e uma conta do painel. A senha nunca sai daqui.
type Usuario struct {
	ID           string    `json:"id"`
	Nome         string    `json:"nome"`
	Email        string    `json:"email"`
	Papel        string    `json:"papel"`
	PapelNome    string    `json:"papel_nome"`
	Ativo        bool      `json:"ativo"`
	TrocarSenha  bool      `json:"trocar_senha"`
	CriadoEm     time.Time `json:"criado_em"`
	UltimoAcesso time.Time `json:"ultimo_acesso"`
	senha        string
}

type chaveUsuario struct{}

// usuarioDe devolve quem fez o pedido (nil se ninguem estiver logado).
func usuarioDe(r *http.Request) *Usuario {
	u, _ := r.Context().Value(chaveUsuario{}).(*Usuario)
	return u
}

// nomeDe devolve o nome de quem fez o pedido. E o que fica no historico da conversa.
func nomeDe(r *http.Request) string {
	if u := usuarioDe(r); u != nil {
		return nomeAtendente(u.Nome)
	}
	return ""
}

// ---------- senha ----------

const (
	argonTempo   = 2
	argonMemoria = 19 * 1024 // em KiB
	argonLinhas  = 1
	argonTamanho = 32
)

// gerarHash transforma a senha em um codigo que nao da para desfazer (Argon2id com
// sal proprio). E so isso que fica gravado.
func gerarHash(senha string) (string, error) {
	sal := make([]byte, 16)
	if _, err := crand.Read(sal); err != nil {
		return "", err
	}
	h := argon2.IDKey([]byte(senha), sal, argonTempo, argonMemoria, argonLinhas, argonTamanho)
	b64 := base64.RawStdEncoding.EncodeToString
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s", argon2.Version, argonMemoria, argonTempo, argonLinhas, b64(sal), b64(h)), nil
}

func conferirHash(senha, guardado string) bool {
	partes := strings.Split(guardado, "$")
	if len(partes) != 6 || partes[1] != "argon2id" {
		return false
	}
	var versao int
	var memoria, tempo uint32
	var linhas uint8
	if _, err := fmt.Sscanf(partes[2], "v=%d", &versao); err != nil || versao != argon2.Version {
		return false
	}
	if _, err := fmt.Sscanf(partes[3], "m=%d,t=%d,p=%d", &memoria, &tempo, &linhas); err != nil {
		return false
	}
	sal, err1 := base64.RawStdEncoding.DecodeString(partes[4])
	esperado, err2 := base64.RawStdEncoding.DecodeString(partes[5])
	if err1 != nil || err2 != nil || len(sal) < 8 || len(esperado) < 16 || len(esperado) > 64 ||
		memoria < 8 || memoria > 1<<20 || tempo < 1 || tempo > 10 || linhas < 1 || linhas > 8 {
		return false
	}
	h := argon2.IDKey([]byte(senha), sal, tempo, memoria, linhas, uint32(len(esperado)))
	return subtle.ConstantTimeCompare(h, esperado) == 1
}

// hashDeMentira e conferido quando o e-mail nao existe, para a resposta demorar o
// mesmo tanto e nao entregar quais e-mails tem conta.
var hashDeMentira, _ = gerarHash("ninguém usa esta senha")

var senhasComuns = map[string]bool{
	"12345678": true, "123456789": true, "1234567890": true, "87654321": true, "11111111": true, "00000000": true,
	"password": true, "password1": true, "qwertyui": true, "qwerty123": true, "abcdefgh": true, "abc12345": true,
	"senha123": true, "senha1234": true, "senha12345": true, "minhasenha": true, "mudar123": true, "trocar123": true,
	"admin123": true, "admin1234": true, "administrador": true, "atendente": true, "atendente123": true,
	"whatsapp": true, "whatsapp123": true, "brasil123": true, "iloveyou": true, "eutenho8": true,
}

// senhaAceita devolve "" quando a senha serve, ou o motivo de nao servir.
func senhaAceita(senha, email, nome string) string {
	if utf8.RuneCountInString(senha) < minSenha {
		return fmt.Sprintf("A senha precisa ter pelo menos %d caracteres.", minSenha)
	}
	if len(senha) > maxSenha {
		return fmt.Sprintf("A senha pode ter no máximo %d caracteres.", maxSenha)
	}
	baixa := strings.ToLower(senha)
	iguais := true
	for _, r := range baixa {
		iguais = iguais && r == []rune(baixa)[0]
	}
	if iguais || senhasComuns[baixa] {
		return "Essa senha é fácil demais de adivinhar. Escolha outra."
	}
	usuario := strings.ToLower(email)
	if i := strings.IndexByte(usuario, '@'); i > 0 {
		usuario = usuario[:i]
	}
	if baixa == strings.ToLower(email) || (len(usuario) >= 4 && strings.Contains(baixa, usuario)) ||
		(len(strings.TrimSpace(nome)) >= 4 && baixa == strings.ToLower(strings.TrimSpace(nome))) {
		return "Não use o seu e-mail nem o seu nome como senha."
	}
	return ""
}

// emailLimpo devolve o e-mail em minusculas, ou "" se nao parecer um e-mail.
func emailLimpo(s string) string {
	e := strings.ToLower(strings.TrimSpace(s))
	arroba := strings.LastIndexByte(e, '@')
	if len(e) > 254 || arroba < 1 || arroba != strings.IndexByte(e, '@') || strings.ContainsAny(e, " \t\r\n<>\"'(),;") {
		return ""
	}
	dominio := e[arroba+1:]
	if ponto := strings.LastIndexByte(dominio, '.'); ponto < 1 || ponto > len(dominio)-3 {
		return ""
	}
	return e
}

func nomeDePessoa(s string) string {
	return cortar(umaLinha(s), 60)
}

// ---------- tentativas de entrada ----------

// portaria conta as senhas erradas. Depois de algumas seguidas para o mesmo e-mail,
// segura as proximas tentativas por um tempo.
type portaria struct {
	mu     sync.Mutex
	falhas map[string][]time.Time
}

func novaPortaria() *portaria {
	return &portaria{falhas: map[string][]time.Time{}}
}

func (p *portaria) recentes(chave string, agora time.Time) []time.Time {
	lista := p.falhas[chave]
	i := 0
	for i < len(lista) && agora.Sub(lista[i]) > janelaFalhas {
		i++
	}
	lista = lista[i:]
	if len(lista) == 0 {
		delete(p.falhas, chave)
	} else {
		p.falhas[chave] = lista
	}
	return lista
}

// espera diz quanto falta para o e-mail poder tentar de novo (0 = pode tentar).
func (p *portaria) espera(email string, agora time.Time) time.Duration {
	p.mu.Lock()
	defer p.mu.Unlock()
	maior := time.Duration(0)
	for chave, limite := range map[string]int{"e:" + email: maxFalhas, "geral": maxFalhasGeral} {
		lista := p.recentes(chave, agora)
		if len(lista) >= limite {
			if falta := janelaFalhas - agora.Sub(lista[len(lista)-limite]); falta > maior {
				maior = falta
			}
		}
	}
	return maior
}

func (p *portaria) falhou(email string, agora time.Time) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.falhas) > 2000 { // alguem enchendo de e-mails inventados: joga fora os vencidos
		for chave := range p.falhas {
			p.recentes(chave, agora)
		}
	}
	for _, chave := range []string{"e:" + email, "geral"} {
		p.falhas[chave] = append(p.recentes(chave, agora), agora)
	}
}

func (p *portaria) acertou(email string) {
	p.mu.Lock()
	delete(p.falhas, "e:"+email)
	p.mu.Unlock()
}

// ---------- usuarios no banco ----------

const colunasUsuario = `id, nome, email, papel, senha, ativo, trocar_senha, criado_em, ultimo_acesso`

func lerUsuario(ler func(dest ...any) error, extras ...any) (*Usuario, error) {
	u := &Usuario{}
	var criado, ultimo string
	dest := append([]any{&u.ID, &u.Nome, &u.Email, &u.Papel, &u.senha, &u.Ativo, &u.TrocarSenha, &criado, &ultimo}, extras...)
	if err := ler(dest...); err != nil {
		return nil, err
	}
	u.CriadoEm, u.UltimoAcesso = lerHora(criado), lerHora(ultimo)
	u.PapelNome = nomesDePapel[u.Papel]
	return u, nil
}

func (b *Banco) totalUsuarios() int {
	var n int
	_ = b.db.QueryRow(`SELECT COUNT(*) FROM usuarios`).Scan(&n)
	return n
}

func (b *Banco) usuarioPor(coluna, valor string) *Usuario {
	if coluna != "id" && coluna != "email" && coluna != "papel" {
		return nil
	}
	u, err := lerUsuario(b.db.QueryRow(`SELECT `+colunasUsuario+` FROM usuarios WHERE `+coluna+` = ? ORDER BY criado_em LIMIT 1`, valor).Scan)
	if err != nil {
		return nil
	}
	return u
}

func (b *Banco) listarUsuarios() ([]*Usuario, error) {
	linhas, err := b.db.Query(`SELECT ` + colunasUsuario + ` FROM usuarios ORDER BY criado_em, id`)
	if err != nil {
		return nil, err
	}
	defer linhas.Close()
	var lista []*Usuario
	for linhas.Next() {
		u, err := lerUsuario(linhas.Scan)
		if err != nil {
			return nil, err
		}
		lista = append(lista, u)
	}
	return lista, linhas.Err()
}

var errEmailEmUso = errors.New("já existe uma conta com esse e-mail")
var errJaTemDono = errors.New("o painel já tem um super administrador")

// criarUsuario grava uma conta nova. O super administrador so pode ser criado
// enquanto nao existir nenhuma conta.
func (b *Banco) criarUsuario(nome, email, papel, hash string, trocarSenha bool) (*Usuario, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if papel == papelSuper && b.totalUsuarios() > 0 {
		return nil, errJaTemDono
	}
	var existe int
	_ = b.db.QueryRow(`SELECT COUNT(*) FROM usuarios WHERE email = ?`, email).Scan(&existe)
	if existe > 0 {
		return nil, errEmailEmUso
	}
	u := &Usuario{ID: novoID("u"), Nome: nome, Email: email, Papel: papel, PapelNome: nomesDePapel[papel], Ativo: true, TrocarSenha: trocarSenha, CriadoEm: time.Now()}
	_, err := b.db.Exec(`INSERT INTO usuarios (id, nome, email, papel, senha, ativo, trocar_senha, criado_em) VALUES (?, ?, ?, ?, ?, 1, ?, ?)`,
		u.ID, u.Nome, u.Email, u.Papel, hash, trocarSenha, horaExata(u.CriadoEm))
	if err != nil {
		return nil, err
	}
	return u, nil
}

func (b *Banco) executar(consulta string, args ...any) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	_, err := b.db.Exec(consulta, args...)
	return err
}

// definirSenha troca a senha e encerra as outras sessoes da pessoa.
func (b *Banco) definirSenha(usuarioID, hash string, trocarDepois bool, manterSessao string) error {
	if err := b.executar(`UPDATE usuarios SET senha = ?, trocar_senha = ? WHERE id = ?`, hash, trocarDepois, usuarioID); err != nil {
		return err
	}
	return b.executar(`DELETE FROM sessoes WHERE usuario_id = ? AND chave <> ?`, usuarioID, chaveDaSessao(manterSessao))
}

// ---------- sessoes ----------

// chaveDaSessao e o que fica no banco: o codigo do navegador passado por SHA-256.
// Quem copiar o banco nao consegue usar as sessoes abertas.
func chaveDaSessao(codigo string) string {
	if codigo == "" {
		return ""
	}
	soma := sha256.Sum256([]byte(codigo))
	return hex.EncodeToString(soma[:])
}

func (b *Banco) criarSessao(usuarioID, navegador string, agora time.Time) (string, error) {
	bruto := make([]byte, 32)
	if _, err := crand.Read(bruto); err != nil {
		return "", err
	}
	codigo := hex.EncodeToString(bruto)
	err := b.executar(`INSERT INTO sessoes (chave, usuario_id, criada_em, vista_em, navegador) VALUES (?, ?, ?, ?, ?)`,
		chaveDaSessao(codigo), usuarioID, horaFixa(agora), horaFixa(agora), cortar(umaLinha(navegador), 200))
	if err != nil {
		return "", err
	}
	_ = b.executar(`UPDATE usuarios SET ultimo_acesso = ? WHERE id = ?`, horaExata(agora), usuarioID)
	return codigo, nil
}

// usuarioDaSessao acha de quem e o codigo que veio do navegador, se a sessao ainda vale.
func (b *Banco) usuarioDaSessao(codigo string, agora time.Time) *Usuario {
	if len(codigo) != 64 {
		return nil
	}
	chave := chaveDaSessao(codigo)
	var criada, vista string
	u, err := lerUsuario(b.db.QueryRow(`SELECT u.id, u.nome, u.email, u.papel, u.senha, u.ativo, u.trocar_senha, u.criado_em, u.ultimo_acesso, s.criada_em, s.vista_em
		FROM sessoes s JOIN usuarios u ON u.id = s.usuario_id WHERE s.chave = ?`, chave).Scan, &criada, &vista)
	if err != nil {
		return nil
	}
	criadaEm, vistaEm := lerHora(criada), lerHora(vista)
	if !u.Ativo || agora.Sub(criadaEm) > sessaoMaxima || agora.Sub(vistaEm) > sessaoParada {
		_ = b.executar(`DELETE FROM sessoes WHERE chave = ?`, chave)
		return nil
	}
	if agora.Sub(vistaEm) > time.Minute {
		_ = b.executar(`UPDATE sessoes SET vista_em = ? WHERE chave = ?`, horaFixa(agora), chave)
	}
	return u
}

func (b *Banco) limparSessoesVelhas(agora time.Time) {
	_ = b.executar(`DELETE FROM sessoes WHERE criada_em < ? OR vista_em < ?`, horaFixa(agora.Add(-sessaoMaxima)), horaFixa(agora.Add(-sessaoParada)))
}

// ---------- registro de acessos ----------

var textosDeAcao = map[string]string{
	"conta_criada":          "Criou a conta de super administrador",
	"entrou":                "Entrou no painel",
	"senha_errada":          "Errou a senha",
	"entrada_bloqueada":     "Entrada bloqueada por muitas tentativas",
	"saiu":                  "Saiu do painel",
	"senha_trocada":         "Trocou a própria senha",
	"senha_redefinida":      "Criou uma senha nova pela recuperação",
	"recuperacao_aberta":    "Abriu a recuperação de senha neste computador",
	"usuario_criado":        "Criou uma conta",
	"usuario_alterado":      "Alterou uma conta",
	"usuario_removido":      "Removeu uma conta",
	"dados_salvos":          "Salvou os dados e ajustes",
	"atendente_ligado":      "Ligou o atendente",
	"atendente_pausado":     "Pausou o atendente",
	"whatsapp_desconectado": "Desconectou o WhatsApp",
	"conversa_assumida":     "Iniciou um atendimento",
	"conversa_devolvida":    "Devolveu uma conversa ao assistente",
	"conversa_apagada":      "Apagou uma conversa",
	"contatos_baixados":     "Baixou uma planilha",
	"texto_alterado":        "Alterou um texto do assistente",
	"material_enviado":      "Adicionou um material",
	"material_removido":     "Removeu um material",
	"ia_login_iniciado":     "Abriu o login do Claude",
	"ia_conectada":          "Conectou a IA",
	"ia_openai_ativada":     "Passou a usar o ChatGPT",
	"ia_claude_ativado":     "Voltou a usar o Claude",
	"ia_chave_removida":     "Removeu a chave da OpenAI",
	"etiqueta_renomeada":    "Renomeou ou juntou uma etiqueta",
	"etiqueta_apagada":      "Apagou uma etiqueta de todos os contatos",
}

// registrar anota quem fez o que. Aparece em Ajustes, para o super administrador.
func (a *App) registrar(u *Usuario, acao, detalhe string) {
	if a.banco == nil {
		return
	}
	id, quem := "", ""
	if u != nil {
		id, quem = u.ID, u.Nome+" ("+u.Email+")"
	}
	if err := a.banco.executar(`INSERT INTO registro (quando, usuario_id, usuario, acao, detalhe) VALUES (?, ?, ?, ?, ?)`,
		horaFixa(time.Now()), id, quem, acao, cortar(umaLinha(detalhe), 300)); err != nil {
		log.Printf("AVISO: não anotei no registro de acessos: %v", err)
	}
}

type linhaRegistro struct {
	N       int64     `json:"n"`
	Quando  time.Time `json:"quando"`
	Usuario string    `json:"usuario"`
	Acao    string    `json:"acao"`
	Texto   string    `json:"texto"`
	Detalhe string    `json:"detalhe"`
}

func (b *Banco) lerRegistro(antes int64, limite int) ([]linhaRegistro, bool, error) {
	if antes <= 0 {
		antes = 1 << 62
	}
	linhas, err := b.db.Query(`SELECT n, quando, usuario, acao, detalhe FROM registro WHERE n < ? ORDER BY n DESC LIMIT ?`, antes, limite+1)
	if err != nil {
		return nil, false, err
	}
	defer linhas.Close()
	lista := []linhaRegistro{}
	for linhas.Next() {
		var l linhaRegistro
		var quando string
		if err := linhas.Scan(&l.N, &quando, &l.Usuario, &l.Acao, &l.Detalhe); err != nil {
			return nil, false, err
		}
		l.Quando = lerHora(quando)
		l.Texto = ouPadrao(textosDeAcao[l.Acao], l.Acao)
		lista = append(lista, l)
	}
	temMais := len(lista) > limite
	if temMais {
		lista = lista[:limite]
	}
	return lista, temMais, linhas.Err()
}

// apararRegistro mantem so as linhas mais novas do registro, para ele nao crescer sem fim.
func (b *Banco) apararRegistro(manter int) {
	_ = b.executar(`DELETE FROM registro WHERE n <= (SELECT COALESCE(MAX(n), 0) FROM registro) - ?`, manter)
}

// ---------- quem esta pedindo ----------

func (a *App) codigoDoPedido(r *http.Request) string {
	c, err := r.Cookie(a.cookie)
	if err != nil {
		return ""
	}
	return c.Value
}

func (a *App) darCookie(w http.ResponseWriter, codigo string) {
	c := &http.Cookie{Name: a.cookie, Value: codigo, Path: "/", HttpOnly: true, SameSite: http.SameSiteStrictMode, MaxAge: int(sessaoMaxima.Seconds())}
	if codigo == "" {
		c.MaxAge = -1
	}
	http.SetCookie(w, c)
}

func recusar(w http.ResponseWriter, r *http.Request, codigo int, msg string, extras map[string]any) {
	if !strings.HasPrefix(r.URL.Path, "/api/") {
		http.Error(w, msg, codigo)
		return
	}
	resp := map[string]any{"ok": false, "erro": msg}
	for k, v := range extras {
		resp[k] = v
	}
	responderJSON(w, codigo, resp)
}

// conferirAcesso descobre quem esta pedindo e se pode. Devolve o pedido com o
// usuario dentro, ou nil se ja respondeu recusando.
func (a *App) conferirAcesso(w http.ResponseWriter, r *http.Request) *http.Request {
	nivel := nivelDaRota(r.Method, r.URL.Path)
	if a.banco == nil {
		if nivel == nivelLivre {
			return r
		}
		recusar(w, r, http.StatusServiceUnavailable, "O painel ainda está iniciando. Tente de novo em instantes.", nil)
		return nil
	}
	u := a.banco.usuarioDaSessao(a.codigoDoPedido(r), time.Now())
	if u != nil {
		r = r.WithContext(context.WithValue(r.Context(), chaveUsuario{}, u))
	}
	if nivel == nivelLivre {
		return r
	}
	if u == nil {
		if a.banco.totalUsuarios() == 0 {
			recusar(w, r, http.StatusUnauthorized, "Crie a conta do super administrador para começar.", map[string]any{"precisa_criar": true})
		} else {
			recusar(w, r, http.StatusUnauthorized, "Entre no painel para continuar.", map[string]any{"entrar": true})
		}
		return nil
	}
	if u.TrocarSenha && r.URL.Path != "/api/senha" {
		recusar(w, r, http.StatusForbidden, "Crie a sua senha antes de continuar.", map[string]any{"trocar_senha": true})
		return nil
	}
	if nivelDoPapel(u.Papel) < nivel {
		recusar(w, r, http.StatusForbidden, "O seu acesso não permite isso. Fale com o administrador do painel.", map[string]any{"sem_permissao": true})
		return nil
	}
	return r
}

// redefinicaoAberta diz se alguem rodou "./atendente -esqueci-senha" ha pouco.
func (a *App) redefinicaoAberta(agora time.Time) bool {
	ate := lerHora(a.banco.meta("redefinir_ate"))
	return !ate.IsZero() && agora.Before(ate)
}

// ---------- rotas ----------

func (a *App) rotaSessao(w http.ResponseWriter, r *http.Request) {
	resp := map[string]any{"versao": versao, "precisa_criar": false, "redefinir": false, "usuario": nil, "min_senha": minSenha}
	if a.banco == nil {
		responderJSON(w, http.StatusOK, resp)
		return
	}
	if u := usuarioDe(r); u != nil {
		resp["usuario"] = u
	} else if a.banco.totalUsuarios() == 0 {
		resp["precisa_criar"] = true
	} else if a.redefinicaoAberta(time.Now()) {
		resp["redefinir"] = true
		if dono := a.banco.usuarioPor("papel", papelSuper); dono != nil {
			resp["redefinir_nome"] = dono.Nome
		}
	}
	responderJSON(w, http.StatusOK, resp)
}

// abrirSessao cria a sessao, entrega o cookie e devolve a conta.
func (a *App) abrirSessao(w http.ResponseWriter, r *http.Request, u *Usuario) bool {
	agora := time.Now()
	codigo, err := a.banco.criarSessao(u.ID, r.UserAgent(), agora)
	if err != nil {
		log.Printf("ERRO: não consegui criar a sessão: %v", err)
		responderErro(w, http.StatusInternalServerError, "Não consegui abrir a sessão. Tente de novo.")
		return false
	}
	a.darCookie(w, codigo)
	u.UltimoAcesso = agora
	return true
}

type pedidoConta struct {
	Nome   string `json:"nome"`
	Email  string `json:"email"`
	Senha  string `json:"senha"`
	Senha2 string `json:"senha2"`
}

// rotaPrimeiroAcesso cria a conta do super administrador. So funciona enquanto nao
// existir nenhuma conta.
func (a *App) rotaPrimeiroAcesso(w http.ResponseWriter, r *http.Request) {
	var p pedidoConta
	if !lerPedido(w, r, &p) {
		return
	}
	if a.banco.totalUsuarios() > 0 {
		responderErro(w, http.StatusConflict, "A conta do super administrador já foi criada. Entre com o e-mail e a senha.")
		return
	}
	nome, email := nomeDePessoa(p.Nome), emailLimpo(p.Email)
	switch {
	case utf8.RuneCountInString(nome) < 2:
		responderJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "campo": "nome", "erro": "Escreva o seu nome."})
		return
	case email == "":
		responderJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "campo": "email", "erro": "Esse e-mail não parece completo. Confira e tente de novo."})
		return
	case p.Senha != p.Senha2:
		responderJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "campo": "senha2", "erro": "As duas senhas não são iguais. Digite de novo."})
		return
	}
	if motivo := senhaAceita(p.Senha, email, nome); motivo != "" {
		responderJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "campo": "senha", "erro": motivo})
		return
	}
	hash, err := gerarHash(p.Senha)
	if err != nil {
		responderErro(w, http.StatusInternalServerError, "Não consegui guardar a senha. Tente de novo.")
		return
	}
	u, err := a.banco.criarUsuario(nome, email, papelSuper, hash, false)
	if err != nil {
		if errors.Is(err, errJaTemDono) || errors.Is(err, errEmailEmUso) {
			responderErro(w, http.StatusConflict, "A conta do super administrador já foi criada. Entre com o e-mail e a senha.")
			return
		}
		log.Printf("ERRO: não consegui criar a conta: %v", err)
		responderErro(w, http.StatusInternalServerError, "Não consegui criar a conta. Tente de novo.")
		return
	}
	if !a.abrirSessao(w, r, u) {
		return
	}
	a.registrar(u, "conta_criada", "")
	log.Printf("ACESSO: conta do super administrador criada")
	responderOK(w, map[string]any{"usuario": u})
}

func (a *App) rotaEntrar(w http.ResponseWriter, r *http.Request) {
	var p pedidoConta
	if !lerPedido(w, r, &p) {
		return
	}
	agora := time.Now()
	email := emailLimpo(p.Email)
	if email == "" || p.Senha == "" || len(p.Senha) > maxSenha {
		time.Sleep(esperaAposFalha)
		responderErro(w, http.StatusUnauthorized, "E-mail ou senha incorretos.")
		return
	}
	if falta := a.acesso.espera(email, agora); falta > 0 {
		minutos := int(falta.Minutes()) + 1
		responderJSON(w, http.StatusTooManyRequests, map[string]any{"ok": false, "espera_min": minutos,
			"erro": "Muitas tentativas com a senha errada. Espere " + strconv.Itoa(minutos) + " min e tente de novo."})
		return
	}
	u := a.banco.usuarioPor("email", email)
	guardado := hashDeMentira
	if u != nil {
		guardado = u.senha
	}
	certo := conferirHash(p.Senha, guardado)
	if u == nil || !certo || !u.Ativo {
		a.acesso.falhou(email, agora)
		if u != nil {
			a.registrar(u, "senha_errada", "")
			if a.acesso.espera(email, agora) > 0 {
				a.registrar(u, "entrada_bloqueada", "")
			}
		} else {
			a.registrar(nil, "senha_errada", "E-mail sem conta: "+cortar(email, 80))
		}
		log.Printf("ACESSO: tentativa de entrada recusada")
		time.Sleep(esperaAposFalha)
		responderErro(w, http.StatusUnauthorized, "E-mail ou senha incorretos.")
		return
	}
	a.acesso.acertou(email)
	if !a.abrirSessao(w, r, u) {
		return
	}
	a.registrar(u, "entrou", "")
	log.Printf("ACESSO: %s entrou no painel", u.ID)
	responderOK(w, map[string]any{"usuario": u})
}

func (a *App) rotaSair(w http.ResponseWriter, r *http.Request) {
	var p struct{}
	if !lerPedido(w, r, &p) {
		return
	}
	if codigo := a.codigoDoPedido(r); codigo != "" && a.banco != nil {
		_ = a.banco.executar(`DELETE FROM sessoes WHERE chave = ?`, chaveDaSessao(codigo))
	}
	if u := usuarioDe(r); u != nil {
		a.registrar(u, "saiu", "")
	}
	a.darCookie(w, "")
	responderOK(w, nil)
}

// rotaSenha troca a senha de quem esta logado.
func (a *App) rotaSenha(w http.ResponseWriter, r *http.Request) {
	var p struct {
		Atual string `json:"atual"`
		Nova  string `json:"nova"`
		Nova2 string `json:"nova2"`
	}
	if !lerPedido(w, r, &p) {
		return
	}
	u := usuarioDe(r)
	if u == nil {
		responderErro(w, http.StatusUnauthorized, "Entre no painel para continuar.")
		return
	}
	if falta := a.acesso.espera(u.Email, time.Now()); falta > 0 {
		responderErro(w, http.StatusTooManyRequests, "Muitas tentativas com a senha errada. Espere alguns minutos e tente de novo.")
		return
	}
	if !conferirHash(p.Atual, u.senha) {
		a.acesso.falhou(u.Email, time.Now())
		time.Sleep(esperaAposFalha)
		responderJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "campo": "atual", "erro": "A senha atual não confere."})
		return
	}
	if p.Nova != p.Nova2 {
		responderJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "campo": "nova2", "erro": "As duas senhas novas não são iguais. Digite de novo."})
		return
	}
	if p.Nova == p.Atual {
		responderJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "campo": "nova", "erro": "A senha nova precisa ser diferente da atual."})
		return
	}
	if motivo := senhaAceita(p.Nova, u.Email, u.Nome); motivo != "" {
		responderJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "campo": "nova", "erro": motivo})
		return
	}
	hash, err := gerarHash(p.Nova)
	if err == nil {
		err = a.banco.definirSenha(u.ID, hash, false, a.codigoDoPedido(r))
	}
	if err != nil {
		log.Printf("ERRO: não consegui trocar a senha: %v", err)
		responderErro(w, http.StatusInternalServerError, "Não consegui trocar a senha. Tente de novo.")
		return
	}
	a.registrar(u, "senha_trocada", "")
	u.TrocarSenha = false
	responderOK(w, map[string]any{"usuario": u})
}

// rotaRedefinir cria uma senha nova para o super administrador. So funciona por
// alguns minutos depois de alguem rodar "./atendente -esqueci-senha" neste computador.
func (a *App) rotaRedefinir(w http.ResponseWriter, r *http.Request) {
	var p pedidoConta
	if !lerPedido(w, r, &p) {
		return
	}
	if !a.redefinicaoAberta(time.Now()) {
		responderErro(w, http.StatusForbidden, "A recuperação não está aberta. Peça de novo e tente em seguida.")
		return
	}
	dono := a.banco.usuarioPor("papel", papelSuper)
	if dono == nil {
		responderErro(w, http.StatusConflict, "Ainda não existe conta para recuperar.")
		return
	}
	if p.Senha != p.Senha2 {
		responderJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "campo": "senha2", "erro": "As duas senhas não são iguais. Digite de novo."})
		return
	}
	if motivo := senhaAceita(p.Senha, dono.Email, dono.Nome); motivo != "" {
		responderJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "campo": "senha", "erro": motivo})
		return
	}
	hash, err := gerarHash(p.Senha)
	if err == nil {
		err = a.banco.definirSenha(dono.ID, hash, false, "")
	}
	if err == nil {
		err = a.banco.executar(`UPDATE usuarios SET ativo = 1 WHERE id = ?`, dono.ID)
	}
	if err != nil {
		log.Printf("ERRO: não consegui redefinir a senha: %v", err)
		responderErro(w, http.StatusInternalServerError, "Não consegui guardar a senha nova. Tente de novo.")
		return
	}
	_ = a.banco.definirMeta("redefinir_ate", "")
	a.acesso.acertou(dono.Email)
	dono.TrocarSenha = false
	if !a.abrirSessao(w, r, dono) {
		return
	}
	a.registrar(dono, "senha_redefinida", "")
	log.Printf("ACESSO: senha do super administrador redefinida pela recuperação")
	responderOK(w, map[string]any{"usuario": dono})
}

// rotaUsuarios lista a equipe (GET) ou cria uma conta (POST). So o super administrador.
func (a *App) rotaUsuarios(w http.ResponseWriter, r *http.Request) {
	eu := usuarioDe(r)
	if r.Method == http.MethodGet {
		lista, err := a.banco.listarUsuarios()
		if err != nil {
			responderErro(w, http.StatusInternalServerError, "Não consegui ler a equipe. Tente de novo.")
			return
		}
		type visao struct {
			*Usuario
			Voce bool `json:"voce"`
		}
		saida := make([]visao, 0, len(lista))
		for _, u := range lista {
			saida = append(saida, visao{u, eu != nil && u.ID == eu.ID})
		}
		responderJSON(w, http.StatusOK, map[string]any{"usuarios": saida, "papeis": []map[string]string{
			{"id": papelAdmin, "nome": nomesDePapel[papelAdmin], "dica": "Pode tudo, menos cuidar da equipe e ver o registro de acessos."},
			{"id": papelAtendente, "nome": nomesDePapel[papelAtendente], "dica": "Atende as conversas e vê contatos, agenda e relatório."},
		}})
		return
	}
	var p struct {
		pedidoConta
		Papel string `json:"papel"`
	}
	if !lerPedido(w, r, &p) {
		return
	}
	nome, email := nomeDePessoa(p.Nome), emailLimpo(p.Email)
	switch {
	case utf8.RuneCountInString(nome) < 2:
		responderJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "campo": "nome", "erro": "Escreva o nome da pessoa."})
		return
	case email == "":
		responderJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "campo": "email", "erro": "Esse e-mail não parece completo. Confira e tente de novo."})
		return
	case p.Papel != papelAdmin && p.Papel != papelAtendente:
		responderJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "campo": "papel", "erro": "Escolha o tipo de acesso."})
		return
	}
	if motivo := senhaAceita(p.Senha, email, nome); motivo != "" {
		responderJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "campo": "senha", "erro": motivo})
		return
	}
	hash, err := gerarHash(p.Senha)
	if err != nil {
		responderErro(w, http.StatusInternalServerError, "Não consegui guardar a senha. Tente de novo.")
		return
	}
	// A senha escolhida aqui e provisoria: a pessoa cria a propria na primeira entrada.
	u, err := a.banco.criarUsuario(nome, email, p.Papel, hash, true)
	if errors.Is(err, errEmailEmUso) {
		responderJSON(w, http.StatusConflict, map[string]any{"ok": false, "campo": "email", "erro": "Já existe uma conta com esse e-mail."})
		return
	}
	if err != nil {
		log.Printf("ERRO: não consegui criar a conta: %v", err)
		responderErro(w, http.StatusInternalServerError, "Não consegui criar a conta. Tente de novo.")
		return
	}
	a.registrar(eu, "usuario_criado", u.Nome+" ("+u.Email+"), "+nomesDePapel[u.Papel])
	responderOK(w, map[string]any{"usuario": u})
}

func (a *App) rotaAlterarUsuario(w http.ResponseWriter, r *http.Request) {
	var p struct {
		ID    string  `json:"id"`
		Nome  *string `json:"nome"`
		Papel *string `json:"papel"`
		Ativo *bool   `json:"ativo"`
		Senha *string `json:"senha"`
	}
	if !lerPedido(w, r, &p) {
		return
	}
	eu := usuarioDe(r)
	u := a.banco.usuarioPor("id", p.ID)
	if u == nil {
		responderErro(w, http.StatusNotFound, "Não achei essa conta.")
		return
	}
	var mudancas []string
	if p.Nome != nil {
		nome := nomeDePessoa(*p.Nome)
		if utf8.RuneCountInString(nome) < 2 {
			responderJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "campo": "nome", "erro": "Escreva o nome da pessoa."})
			return
		}
		if nome != u.Nome {
			if err := a.banco.executar(`UPDATE usuarios SET nome = ? WHERE id = ?`, nome, u.ID); err != nil {
				responderErro(w, http.StatusInternalServerError, "Não consegui salvar. Tente de novo.")
				return
			}
			mudancas = append(mudancas, "nome")
		}
	}
	if p.Papel != nil && *p.Papel != u.Papel {
		if u.Papel == papelSuper || (*p.Papel != papelAdmin && *p.Papel != papelAtendente) {
			responderErro(w, http.StatusBadRequest, "O acesso do super administrador não pode ser trocado.")
			return
		}
		if err := a.banco.executar(`UPDATE usuarios SET papel = ? WHERE id = ?`, *p.Papel, u.ID); err != nil {
			responderErro(w, http.StatusInternalServerError, "Não consegui salvar. Tente de novo.")
			return
		}
		mudancas = append(mudancas, "acesso: "+nomesDePapel[*p.Papel])
	}
	if p.Ativo != nil && *p.Ativo != u.Ativo {
		if u.Papel == papelSuper || (eu != nil && u.ID == eu.ID) {
			responderErro(w, http.StatusBadRequest, "A conta do super administrador não pode ser desativada.")
			return
		}
		if err := a.banco.executar(`UPDATE usuarios SET ativo = ? WHERE id = ?`, *p.Ativo, u.ID); err != nil {
			responderErro(w, http.StatusInternalServerError, "Não consegui salvar. Tente de novo.")
			return
		}
		if !*p.Ativo {
			_ = a.banco.executar(`DELETE FROM sessoes WHERE usuario_id = ?`, u.ID)
			mudancas = append(mudancas, "conta desativada")
		} else {
			mudancas = append(mudancas, "conta reativada")
		}
	}
	if p.Senha != nil {
		if eu != nil && u.ID == eu.ID {
			responderErro(w, http.StatusBadRequest, "Para trocar a sua senha, use Trocar senha no menu da sua conta.")
			return
		}
		if motivo := senhaAceita(*p.Senha, u.Email, u.Nome); motivo != "" {
			responderJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "campo": "senha", "erro": motivo})
			return
		}
		hash, err := gerarHash(*p.Senha)
		if err == nil {
			err = a.banco.definirSenha(u.ID, hash, true, "")
		}
		if err != nil {
			responderErro(w, http.StatusInternalServerError, "Não consegui guardar a senha. Tente de novo.")
			return
		}
		a.acesso.acertou(u.Email)
		mudancas = append(mudancas, "senha provisória nova")
	}
	if len(mudancas) > 0 {
		a.registrar(eu, "usuario_alterado", u.Nome+" ("+u.Email+"): "+strings.Join(mudancas, ", "))
	}
	responderOK(w, map[string]any{"usuario": a.banco.usuarioPor("id", u.ID)})
}

func (a *App) rotaExcluirUsuario(w http.ResponseWriter, r *http.Request) {
	var p pedidoID
	if !lerPedido(w, r, &p) {
		return
	}
	eu := usuarioDe(r)
	u := a.banco.usuarioPor("id", p.ID)
	if u == nil {
		responderErro(w, http.StatusNotFound, "Não achei essa conta.")
		return
	}
	if u.Papel == papelSuper || (eu != nil && u.ID == eu.ID) {
		responderErro(w, http.StatusBadRequest, "A conta do super administrador não pode ser removida.")
		return
	}
	if err := a.banco.executar(`DELETE FROM usuarios WHERE id = ?`, u.ID); err != nil {
		responderErro(w, http.StatusInternalServerError, "Não consegui remover a conta. Tente de novo.")
		return
	}
	a.registrar(eu, "usuario_removido", u.Nome+" ("+u.Email+")")
	responderOK(w, nil)
}

func (a *App) rotaRegistro(w http.ResponseWriter, r *http.Request) {
	antes, _ := strconv.ParseInt(r.URL.Query().Get("antes"), 10, 64)
	limite, err := strconv.Atoi(r.URL.Query().Get("limite"))
	if err != nil {
		limite = 50
	}
	lista, temMais, err := a.banco.lerRegistro(antes, limitar(limite, 1, 200))
	if err != nil {
		responderErro(w, http.StatusInternalServerError, "Não consegui ler o registro. Tente de novo.")
		return
	}
	responderJSON(w, http.StatusOK, map[string]any{"registros": lista, "tem_mais": temMais})
}

// abrirRedefinicao e o que "./atendente -esqueci-senha" faz: abre, por alguns
// minutos, a tela de criar uma senha nova para o super administrador.
func abrirRedefinicao() int {
	if !existeArquivo(arqBanco) {
		fmt.Println("Ainda não existe conta neste painel. Abra o painel no navegador e crie a conta do super administrador.")
		return 1
	}
	banco, err := abrirBanco(arqBanco)
	if err != nil {
		fmt.Println("Não consegui abrir o banco de dados:", err)
		return 1
	}
	defer banco.fechar()
	dono := banco.usuarioPor("papel", papelSuper)
	if dono == nil {
		fmt.Println("Ainda não existe conta neste painel. Abra o painel no navegador e crie a conta do super administrador.")
		return 1
	}
	if err := banco.definirMeta("redefinir_ate", horaExata(time.Now().Add(prazoRedefinir))); err != nil {
		fmt.Println("Não consegui abrir a recuperação:", err)
		return 1
	}
	_ = banco.executar(`INSERT INTO registro (quando, usuario_id, usuario, acao, detalhe) VALUES (?, '', '', 'recuperacao_aberta', 'Pedida neste computador')`, horaFixa(time.Now()))
	fmt.Printf("Recuperação aberta por %d minutos.\n", int(prazoRedefinir.Minutes()))
	fmt.Println("Abra o painel no navegador (ou recarregue a página): vai aparecer a tela para criar uma senha nova do super administrador.")
	return 0
}
