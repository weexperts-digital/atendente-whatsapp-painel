package main

import (
	"bytes"
	"context"
	"embed"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	qrcode "github.com/skip2/go-qrcode"
	"go.mau.fi/whatsmeow/types"
)

const portaInicial = 8787

// A pagina (HTML, CSS e JS) vai embutida dentro do programa.
//
//go:embed all:web
var arquivosWeb embed.FS

// rotas junta todos os enderecos do painel.
func (a *App) rotas() *http.ServeMux {
	mux := http.NewServeMux()
	mux.Handle("/", a.rotaArquivos())
	mux.HandleFunc("/qr.png", a.rotaQR)
	mux.HandleFunc("/leads.csv", a.rotaLeadsCSV)
	mux.HandleFunc("/agenda.csv", a.rotaAgendaCSV)
	mux.HandleFunc("/contatos.csv", a.rotaContatosCSV)
	mux.HandleFunc("/api/contatos", a.rotaContatos)
	a.rotasDaFicha(mux) // ficha do contato (contatos.go)
	a.rotasDeVoz(mux)   // reconhecimento de voz local (voz.go)

	mux.HandleFunc("/api/sessao", a.rotaSessao)
	mux.HandleFunc("/api/primeiro-acesso", a.rotaPrimeiroAcesso)
	mux.HandleFunc("/api/entrar", a.rotaEntrar)
	mux.HandleFunc("/api/sair", a.rotaSair)
	mux.HandleFunc("/api/senha", a.rotaSenha)
	mux.HandleFunc("/api/redefinir-senha", a.rotaRedefinir)
	mux.HandleFunc("/api/usuarios", a.rotaUsuarios)
	mux.HandleFunc("/api/usuarios/alterar", a.rotaAlterarUsuario)
	mux.HandleFunc("/api/usuarios/excluir", a.rotaExcluirUsuario)
	mux.HandleFunc("/api/registro", a.rotaRegistro)

	mux.HandleFunc("/api/estado", a.rotaEstado)
	mux.HandleFunc("/api/liga", a.rotaLiga)
	mux.HandleFunc("/api/whatsapp/desconectar", a.rotaDesconectar)

	mux.HandleFunc("/api/lead", a.rotaLead)
	mux.HandleFunc("/api/lead/historico", a.rotaHistorico)
	mux.HandleFunc("/api/retomar", a.rotaRetomar)
	mux.HandleFunc("/api/assumir", a.rotaAssumir)
	mux.HandleFunc("/api/responder", a.rotaResponder)
	mux.HandleFunc("/api/lead/excluir", a.rotaExcluirLead)
	mux.HandleFunc("/api/lead/etiquetas", a.rotaEtiquetas)
	mux.HandleFunc("/api/etiquetas", a.rotaCatalogoDeEtiquetas)
	mux.HandleFunc("/api/etiquetas/renomear", a.rotaRenomearEtiqueta)
	mux.HandleFunc("/api/etiquetas/excluir", a.rotaExcluirEtiqueta)
	mux.HandleFunc("/api/enviar-arquivo", a.rotaEnviarArquivo)
	mux.HandleFunc("/api/midia", a.rotaMidia)
	mux.HandleFunc("/api/foto", a.rotaFoto)
	mux.HandleFunc("/api/relatorio", a.rotaRelatorio)

	mux.HandleFunc("/api/config", a.rotaConfig)
	mux.HandleFunc("/api/textos", a.rotaTextos)
	mux.HandleFunc("/api/textos/restaurar", a.rotaRestaurarTexto)

	mux.HandleFunc("/api/ia/negocio", a.rotaIANegocio)
	mux.HandleFunc("/api/ia/melhorar", a.rotaIAMelhorar)
	mux.HandleFunc("/api/ia/cerebro", a.rotaIACerebro)
	mux.HandleFunc("/api/ia/conexao", a.rotaConexao)
	mux.HandleFunc("/api/ia/conectar", a.rotaConectar)
	mux.HandleFunc("/api/ia/conectar/codigo", a.rotaCodigoLogin)
	mux.HandleFunc("/api/ia/conectar/cancelar", a.rotaCancelarLogin)
	mux.HandleFunc("/api/ia/openai", a.rotaOpenAI)
	mux.HandleFunc("/api/ia/openai/modelos", a.rotaModelosOpenAI)
	mux.HandleFunc("/api/ia/openai/remover", a.rotaRemoverOpenAI)
	mux.HandleFunc("/api/ia/provedor", a.rotaProvedor)
	mux.HandleFunc("/api/treinar", a.rotaTreinar)
	mux.HandleFunc("/api/aprendizado", a.rotaAprendizado)

	mux.HandleFunc("/api/conhecimento", a.rotaConhecimento)
	mux.HandleFunc("/api/conhecimento/enviar", a.rotaEnviarMaterial)
	mux.HandleFunc("/api/conhecimento/texto", a.rotaTextoMaterial)
	mux.HandleFunc("/api/conhecimento/alterar", a.rotaAlterarMaterial)
	mux.HandleFunc("/api/conhecimento/excluir", a.rotaExcluirMaterial)

	mux.HandleFunc("/api/agenda", a.rotaAgenda)
	mux.HandleFunc("/api/agenda/marcar", a.rotaMarcar)
	mux.HandleFunc("/api/agenda/cancelar", a.rotaCancelar)
	return mux
}

// iniciarPainel sobe o servidor local. Escuta SO em 127.0.0.1.
func (a *App) iniciarPainel() (int, error) {
	mux := a.rotas()
	var escuta net.Listener
	porta := 0
	// ATENDENTE_PORTA troca a primeira porta tentada (para rodar uma copia de teste ao lado).
	inicial := portaInicial
	if v, err := strconv.Atoi(os.Getenv("ATENDENTE_PORTA")); err == nil && v > 1024 && v < 65000 {
		inicial = v
	}
	for i := 0; i <= 20; i++ {
		p := inicial + i
		l, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", p))
		if err == nil {
			escuta, porta = l, p
			break
		}
	}
	if escuta == nil {
		return 0, fmt.Errorf("nenhuma porta livre entre %d e %d", inicial, inicial+20)
	}
	a.mu.Lock()
	a.porta = porta
	// Cada painel tem o seu cookie: uma copia de teste em outra porta nao derruba a sessao desta.
	a.cookie = "atendente_" + strconv.Itoa(porta)
	a.mu.Unlock()
	if err := os.WriteFile("porta.txt", []byte(strconv.Itoa(porta)), 0o644); err != nil {
		log.Printf("AVISO: não gravei porta.txt: %v", err)
	}
	log.Printf("PAINEL http://localhost:%d", porta)

	srv := &http.Server{Handler: a.protegido(mux), ReadHeaderTimeout: 10 * time.Second}
	go func() {
		if err := srv.Serve(escuta); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("ERRO no painel: %v", err)
		}
	}()
	return porta, nil
}

func hostLocal(h string) bool {
	return h == "localhost" || h == "127.0.0.1"
}

const (
	rotaDeArquivo = "/api/conhecimento/enviar"
	rotaDeMidia   = "/api/enviar-arquivo"
)

// protegido barra pedidos que nao venham da propria pagina local.
func (a *App) protegido(proximo http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nome := r.Host
		if h, _, err := net.SplitHostPort(r.Host); err == nil {
			nome = h
		}
		if !hostLocal(nome) {
			http.Error(w, "acesso só pelo próprio computador", http.StatusForbidden)
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			if r.Method != http.MethodPost {
				http.Error(w, "método não aceito", http.StatusMethodNotAllowed)
				return
			}
			if o := r.Header.Get("Origin"); o != "" {
				u, err := url.Parse(o)
				if err != nil || u.Host != r.Host {
					http.Error(w, "origem não aceita", http.StatusForbidden)
					return
				}
			}
			tipo := r.Header.Get("Content-Type")
			if r.URL.Path == rotaDeArquivo || r.URL.Path == rotaDeMidia {
				if !strings.HasPrefix(tipo, "application/octet-stream") {
					http.Error(w, "formato não aceito", http.StatusUnsupportedMediaType)
					return
				}
				r.Body = http.MaxBytesReader(w, r.Body, maxArquivo+1024)
			} else {
				if !strings.HasPrefix(tipo, "application/json") {
					http.Error(w, "formato não aceito", http.StatusUnsupportedMediaType)
					return
				}
				r.Body = http.MaxBytesReader(w, r.Body, 2<<20)
			}
		}
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		// 'wasm-unsafe-eval' e os workers são do reconhecimento de voz, que roda dentro da
		// página (WebAssembly) com arquivos servidos por este programa. Nada vem de fora.
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self' 'wasm-unsafe-eval'; worker-src 'self' blob:; style-src 'self' 'unsafe-inline'; img-src 'self' data: blob:; media-src 'self' blob:; connect-src 'self' blob: data:; frame-ancestors 'none'; base-uri 'none'; form-action 'self'")
		// Isolamento entre origens: deixa o reconhecimento de voz usar mais de um núcleo.
		w.Header().Set("Cross-Origin-Opener-Policy", "same-origin")
		w.Header().Set("Cross-Origin-Embedder-Policy", "require-corp")
		w.Header().Set("Cross-Origin-Resource-Policy", "same-origin")
		// Daqui para dentro, so quem esta logado e tem permissao para a rota.
		r = a.conferirAcesso(w, r)
		if r == nil {
			return
		}
		proximo.ServeHTTP(w, r)
	})
}

func responderJSON(w http.ResponseWriter, codigo int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(codigo)
	_ = json.NewEncoder(w).Encode(v)
}

func responderErro(w http.ResponseWriter, codigo int, msg string) {
	responderJSON(w, codigo, map[string]any{"ok": false, "erro": msg})
}

func responderOK(w http.ResponseWriter, extras map[string]any) {
	if extras == nil {
		extras = map[string]any{}
	}
	extras["ok"] = true
	responderJSON(w, http.StatusOK, extras)
}

// lerPedido confere que e POST e le o JSON. Devolve falso se ja respondeu com erro.
func lerPedido(w http.ResponseWriter, r *http.Request, destino any) bool {
	if r.Method != http.MethodPost {
		responderErro(w, http.StatusMethodNotAllowed, "Use os botões da página.")
		return false
	}
	if err := json.NewDecoder(r.Body).Decode(destino); err != nil && !errors.Is(err, io.EOF) {
		responderErro(w, http.StatusBadRequest, "Não entendi os dados enviados. Tente de novo.")
		return false
	}
	return true
}

// erroDaIA transforma uma falha do Claude em um aviso que a pessoa entende.
func erroDaIA(w http.ResponseWriter, err error) {
	if errors.Is(err, errSemCerebro) {
		responderErro(w, http.StatusServiceUnavailable, "Esta função precisa do Claude. Instale o Claude Code neste computador e entre na sua conta.")
		return
	}
	log.Printf("IA: pedido do painel falhou: %v", err)
	if aviso := avisoDoCerebro(err.Error()); aviso != "" {
		// conectar_ia faz a página abrir a janela "Conectar a IA".
		responderJSON(w, http.StatusServiceUnavailable, map[string]any{"ok": false, "erro": aviso, "conectar_ia": true})
		return
	}
	msg := "A IA não conseguiu responder agora. Espere um pouco e tente de novo."
	if strings.Contains(err.Error(), "demorou") {
		msg = "A IA demorou demais para responder. Tente de novo, ou com um texto menor."
	} else if strings.Contains(err.Error(), "fora do formato") || strings.Contains(err.Error(), "vazio") {
		msg = err.Error()
		msg = strings.ToUpper(msg[:1]) + msg[1:] + "."
	}
	responderErro(w, http.StatusBadGateway, msg)
}

// rotaArquivos entrega a pagina. ATENDENTE_WEB aponta para uma pasta no disco, para
// quem esta mexendo na pagina e quer ver a mudanca sem compilar de novo.
func (a *App) rotaArquivos() http.Handler {
	var arquivos fs.FS
	if pasta := strings.TrimSpace(os.Getenv("ATENDENTE_WEB")); pasta != "" {
		arquivos = os.DirFS(pasta)
	} else {
		sub, err := fs.Sub(arquivosWeb, "web")
		if err != nil {
			log.Fatalf("ERRO: a página não veio dentro do programa: %v", err)
		}
		arquivos = sub
	}
	servidor := http.FileServer(http.FS(arquivos))
	tipos := map[string]string{
		".js": "text/javascript; charset=utf-8", ".css": "text/css; charset=utf-8",
		".svg": "image/svg+xml", ".html": "text/html; charset=utf-8",
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.Error(w, "método não aceito", http.StatusMethodNotAllowed)
			return
		}
		for fim, tipo := range tipos {
			if strings.HasSuffix(r.URL.Path, fim) {
				w.Header().Set("Content-Type", tipo)
			}
		}
		servidor.ServeHTTP(w, r)
	})
}

func (a *App) rotaQR(w http.ResponseWriter, r *http.Request) {
	a.mu.Lock()
	codigo := a.qr
	a.mu.Unlock()
	if codigo == "" {
		http.Error(w, "sem código QR no momento", http.StatusNotFound)
		return
	}
	png, err := qrcode.Encode(codigo, qrcode.Medium, 360)
	if err != nil {
		http.Error(w, "não consegui gerar o código QR", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "image/png")
	_, _ = w.Write(png)
}

// ---------- estado e conversas ----------

type agendamentoCurto struct {
	Servico string    `json:"servico"`
	Inicio  time.Time `json:"inicio"`
}

type leadVisao struct {
	ID              string            `json:"id"`
	Exibir          string            `json:"exibir"`
	Telefone        string            `json:"telefone"`
	TelefoneBonito  string            `json:"telefone_bonito"`
	Nome            string            `json:"nome"`
	NomeWhats       string            `json:"nome_whatsapp"`
	Necessidade     string            `json:"necessidade"`
	Orcamento       string            `json:"orcamento"`
	Decisor         string            `json:"decisor"`
	Prazo           string            `json:"prazo"`
	Pontuacao       int               `json:"pontuacao"`
	Classificacao   string            `json:"classificacao"`
	Etapa           string            `json:"etapa"`
	Resumo          string            `json:"resumo"`
	Status          string            `json:"status"`
	Motivo          string            `json:"motivo"`
	CriadoEm        time.Time         `json:"criado_em"`
	UltimaMsg       time.Time         `json:"ultima_msg"`
	Previa          string            `json:"previa"`
	PreviaDe        string            `json:"previa_de"`
	TotalMsgs       int               `json:"total_msgs"`
	ProximoAgendado *agendamentoCurto `json:"proximo_agendamento"`
	Etiquetas       []string          `json:"etiquetas"`
	EtiquetasIA     []string          `json:"etiquetas_ia"` // as que a IA criou
	TemFoto         bool              `json:"tem_foto"`
	FotoVersao      string            `json:"foto_versao"`
	Atendente       string            `json:"atendente"`
	NaoLidas        int               `json:"nao_lidas"`
	PreviaTipo      string            `json:"previa_tipo"`
	Historico       []Msg             `json:"historico,omitempty"`
}

// visaoLead prepara um lead para a pagina. Chame com a.mu travado.
func (a *App) visaoLead(l *Lead, agora time.Time) leadVisao {
	v := leadVisao{
		ID: l.ID, Exibir: l.exibir(), Telefone: soDigitos(l.Telefone), TelefoneBonito: telefoneBonito(l.Telefone),
		Nome: l.Nome, NomeWhats: l.NomeWhats, Necessidade: l.Necessidade, Orcamento: l.Orcamento,
		Decisor: l.Decisor, Prazo: l.Prazo, Pontuacao: l.Pontuacao,
		Classificacao: ouPadrao(l.Classificacao, "indefinido"), Etapa: l.Etapa, Resumo: l.Resumo,
		Status: ouPadrao(l.Status, statusQualificando), Motivo: l.Motivo,
		CriadoEm: l.CriadoEm, UltimaMsg: l.UltimaMsg, TotalMsgs: l.totalMensagens(),
	}
	v.Etiquetas = append([]string{}, l.Etiquetas...)
	v.EtiquetasIA = append([]string{}, l.EtiquetasIA...)
	v.Atendente = l.Atendente
	if l.FotoID != "" && existeArquivo(arquivoFoto(l.ID)) {
		v.TemFoto = true
		v.FotoVersao = cortar(l.FotoID, 16)
	}
	// Previa e nao lidas ignoram as linhas de historico interno.
	achouPrevia, contando := false, true
	for i := len(l.Historico) - 1; i >= 0; i-- {
		m := l.Historico[i]
		if m.De == "sistema" {
			continue
		}
		if !achouPrevia {
			achouPrevia = true
			v.Previa = cortar(umaLinha(m.textoPlano()), 90)
			v.PreviaDe = m.De
			v.PreviaTipo = ouPadrao(m.Tipo, "texto")
		}
		if contando && m.De == "cliente" {
			v.NaoLidas++
		} else {
			contando = false
		}
		if !contando {
			break
		}
	}
	if futuros := a.futurosDoLead(l.ID, agora); len(futuros) > 0 {
		v.ProximoAgendado = &agendamentoCurto{Servico: futuros[0].Servico, Inicio: futuros[0].Inicio}
	}
	return v
}

func (a *App) rotaEstado(w http.ResponseWriter, r *http.Request) {
	modo, erroCerebro := a.cerebro.Modo()
	materiais := a.base.total()
	agora := time.Now()
	// Só o que já foi conferido: esta rota é chamada a cada 3 segundos e não pode esperar o claude.
	var ia *situacaoIA
	if e, pronto := a.conexao.guardado(); pronto {
		s := a.situacaoDaIA(e)
		ia = &s
	}

	a.mu.Lock()
	leads := make([]leadVisao, 0, len(a.leads))
	cont := map[string]int{"conversas": 0, "aguardando": 0, "quentes": 0, "mornos": 0, "frios": 0, "agendamentos": 0, "materiais": materiais, "em_atendimento": 0}
	for _, l := range a.leadsOrdenados() {
		cont["conversas"]++
		switch l.Status {
		case statusHumano:
			cont["aguardando"]++
		case statusAtendimento:
			cont["em_atendimento"]++
		}
		switch l.Classificacao {
		case "quente":
			cont["quentes"]++
		case "morno":
			cont["mornos"]++
		case "frio":
			cont["frios"]++
		}
		leads = append(leads, a.visaoLead(l, agora))
	}
	for _, ag := range a.agenda {
		if ag.Status == "marcado" && ag.Inicio.After(agora) {
			cont["agendamentos"]++
		}
	}
	resp := map[string]any{
		"versao":        versao,
		"status":        a.status,
		"mensagem":      a.mensagem,
		"erro":          a.erro,
		"nome":          a.nomeConta,
		"numero":        telefoneBonito(a.numConta),
		"tem_qr":        a.qr != "",
		"salvo":         a.cfg.Salvo,
		"ligado":        a.cfg.Ligado && a.cfg.Salvo,
		"modo":          modo,
		"erro_cerebro":  erroCerebro,
		"aviso_cerebro": avisoDoCerebro(erroCerebro),
		"ia":            ia,
		"modelo":        modeloDe(a.cfg),
		"agenda_ativa":  a.cfg.Agenda.Ativa,
		"leads":         leads,
		"contadores":    cont,
		"etiquetas":     a.etiquetasEmUso(),
	}
	a.mu.Unlock()
	responderJSON(w, http.StatusOK, resp)
}

func (a *App) rotaLiga(w http.ResponseWriter, r *http.Request) {
	var p struct {
		Ligar bool `json:"ligar"`
	}
	if !lerPedido(w, r, &p) {
		return
	}
	a.mu.Lock()
	if p.Ligar && !a.cfg.Salvo {
		a.mu.Unlock()
		responderErro(w, http.StatusBadRequest, "Preencha e salve os dados do negócio antes de ligar.")
		return
	}
	a.cfg.Ligado = p.Ligar
	a.salvarConfig()
	a.mu.Unlock()
	if p.Ligar {
		log.Printf("PAINEL: atendente LIGADO")
		a.registrar(usuarioDe(r), "atendente_ligado", "")
	} else {
		log.Printf("PAINEL: atendente PAUSADO")
		a.registrar(usuarioDe(r), "atendente_pausado", "")
	}
	responderOK(w, map[string]any{"ligado": p.Ligar})
}

func (a *App) rotaDesconectar(w http.ResponseWriter, r *http.Request) {
	var p struct{}
	if !lerPedido(w, r, &p) {
		return
	}
	cli := a.cliente()
	if cli == nil || cli.Store.ID == nil {
		responderErro(w, http.StatusConflict, "Nenhum WhatsApp conectado no momento.")
		return
	}
	ctx, cancelar := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancelar()
	if err := cli.Logout(ctx); err != nil {
		// Sem conexao nao da para avisar o WhatsApp: apaga a sessao local mesmo assim.
		log.Printf("WHATSAPP: não avisei a saída (%v), apagando a sessão local", err)
		cli.Disconnect()
		_ = cli.Store.Delete(ctx)
	}
	log.Printf("PAINEL: aparelho desconectado pelo painel")
	a.registrar(usuarioDe(r), "whatsapp_desconectado", "")
	a.definirStatus("desconectado", "Aparelho desconectado. Gerando um novo código...")
	a.pedirTroca()
	responderOK(w, nil)
}

// leadDoPedido le {"id": "..."} e acha o lead. Devolve com a.mu TRAVADO quando da certo.
func (a *App) leadDoPedido(w http.ResponseWriter, id string) *Lead {
	a.mu.Lock()
	l := a.leads[id]
	if l == nil {
		a.mu.Unlock()
		responderErro(w, http.StatusNotFound, "Não achei essa conversa.")
		return nil
	}
	return l
}

func (a *App) rotaLead(w http.ResponseWriter, r *http.Request) {
	agora := time.Now()
	l := a.leadDoPedido(w, r.URL.Query().Get("id"))
	if l == nil {
		return
	}
	v := a.visaoLead(l, agora)
	v.Historico = make([]Msg, len(l.Historico))
	copy(v.Historico, l.Historico)
	for i := range v.Historico {
		v.Historico[i].Pendente = false
	}
	var ags []agendamentoVisao
	for _, ag := range a.agenda {
		if ag.LeadID == l.ID {
			ags = append(ags, visaoAgendamento(ag))
		}
	}
	id := l.ID
	var primeira int64
	if len(v.Historico) > 0 {
		primeira = v.Historico[0].N
	}
	a.mu.Unlock()
	temAnteriores := primeira > 0 && a.banco != nil && a.banco.temAntes(id, primeira)
	// Ao abrir a conversa, confere a foto do contato (no maximo uma vez por dia).
	if cli := a.cliente(); cli != nil && cli.IsConnected() && cli.IsLoggedIn() {
		go a.atualizarFoto(cli, id)
	}
	sort.Slice(ags, func(i, j int) bool { return ags[i].Inicio.After(ags[j].Inicio) })
	if ags == nil {
		ags = []agendamentoVisao{}
	}
	responderOK(w, map[string]any{"lead": v, "agendamentos": ags, "tem_anteriores": temAnteriores})
}

// rotaHistorico devolve mensagens mais antigas que as que estao na tela. O painel
// guarda so as ultimas em uso; o banco guarda todas.
func (a *App) rotaHistorico(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	antes, err := strconv.ParseInt(r.URL.Query().Get("antes"), 10, 64)
	a.mu.Lock()
	existe := a.leads[id] != nil
	a.mu.Unlock()
	if !existe || err != nil || antes <= 0 || a.banco == nil {
		responderErro(w, http.StatusNotFound, "Não achei essa conversa.")
		return
	}
	msgs, temMais, err := a.banco.mensagensAntes(id, antes, 60)
	if err != nil {
		log.Printf("PAINEL: não li as mensagens antigas: %v", err)
		responderErro(w, http.StatusInternalServerError, "Não consegui buscar as mensagens antigas. Tente de novo.")
		return
	}
	if msgs == nil {
		msgs = []Msg{}
	}
	responderOK(w, map[string]any{"mensagens": msgs, "tem_mais": temMais})
}

type pedidoID struct {
	ID string `json:"id"`
}

// nomeAtendente limpa o nome de quem atende. Ele fica so no historico do painel.
func nomeAtendente(s string) string {
	return cortar(umaLinha(s), 40)
}

func (a *App) rotaRetomar(w http.ResponseWriter, r *http.Request) {
	var p pedidoID
	if !lerPedido(w, r, &p) {
		return
	}
	l := a.leadDoPedido(w, p.ID)
	if l == nil {
		return
	}
	if l.comHumano() {
		a.retomar(l)
		l.evento("devolveu", ouPadrao(nomeDe(r), "A equipe")+" devolveu a conversa ao assistente")
		a.salvarLeads()
	}
	contato := l.exibir()
	a.mu.Unlock()
	log.Printf("PAINEL: conversa devolvida ao assistente")
	a.registrar(usuarioDe(r), "conversa_devolvida", contato)
	responderOK(w, nil)
}

func (a *App) rotaAssumir(w http.ResponseWriter, r *http.Request) {
	var p pedidoID
	if !lerPedido(w, r, &p) {
		return
	}
	l := a.leadDoPedido(w, p.ID)
	if l == nil {
		return
	}
	quem := nomeDe(r)
	iniciou := l.Status != statusAtendimento || (quem != "" && l.Atendente != quem)
	if iniciou {
		a.iniciarAtendimento(l, quem)
		a.salvarLeads()
	}
	contato := l.exibir()
	a.mu.Unlock()
	log.Printf("PAINEL: atendimento iniciado por uma pessoa")
	if iniciou {
		a.registrar(usuarioDe(r), "conversa_assumida", contato)
	}
	responderOK(w, nil)
}

func (a *App) rotaResponder(w http.ResponseWriter, r *http.Request) {
	var p struct {
		ID    string `json:"id"`
		Texto string `json:"texto"`
	}
	if !lerPedido(w, r, &p) {
		return
	}
	texto := cortar(strings.TrimSpace(p.Texto), 4000)
	if texto == "" {
		responderErro(w, http.StatusBadRequest, "Escreva a mensagem antes de enviar.")
		return
	}
	l := a.leadDoPedido(w, p.ID)
	if l == nil {
		return
	}
	a.mu.Unlock()
	chat, err := types.ParseJID(p.ID)
	if err != nil {
		responderErro(w, http.StatusBadRequest, "Não achei essa conversa.")
		return
	}
	if err := a.enviarTexto(chat, texto); err != nil {
		responderErro(w, http.StatusConflict, "Não consegui enviar. Confira se o WhatsApp está conectado.")
		return
	}
	a.guardarRespostaDaEquipe(p.ID, Msg{De: "humano", Texto: texto, Hora: time.Now()}, nomeDe(r))
	log.Printf("PAINEL: resposta enviada por uma pessoa para %s", fimDoNumero(p.ID))
	responderOK(w, nil)
}

// guardarRespostaDaEquipe anota o que a pessoa enviou pelo painel e, se a conversa
// ainda nao estava com ela, registra que ela assumiu.
func (a *App) guardarRespostaDaEquipe(id string, msg Msg, atendente string) {
	quem := nomeAtendente(atendente)
	msg.Autor = quem
	a.mu.Lock()
	defer a.mu.Unlock()
	l := a.leads[id]
	if l == nil {
		return
	}
	if l.Status != statusAtendimento {
		a.iniciarAtendimento(l, quem)
	} else if quem != "" && l.Atendente == "" {
		l.Atendente = quem
	}
	l.anexar(msg)
	l.UltimaMsg = time.Now()
	a.salvarLeads()
}

// rotaEnviarArquivo manda imagem, audio, video ou documento escolhido pela pessoa.
func (a *App) rotaEnviarArquivo(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		responderErro(w, http.StatusMethodNotAllowed, "Use o botão de anexar da página.")
		return
	}
	cab := func(nome string) string {
		v, err := url.QueryUnescape(r.Header.Get(nome))
		if err != nil {
			return ""
		}
		return v
	}
	id := cab("X-Lead")
	dados, err := io.ReadAll(r.Body)
	if err != nil {
		responderErro(w, http.StatusRequestEntityTooLarge, fmt.Sprintf("Este arquivo passa de %d MB. Envie um arquivo menor.", maxMidia>>20))
		return
	}
	if len(dados) == 0 {
		responderErro(w, http.StatusBadRequest, "O arquivo chegou vazio.")
		return
	}
	a.mu.Lock()
	existe := a.leads[id] != nil
	a.mu.Unlock()
	chat, errJID := types.ParseJID(id)
	if !existe || errJID != nil {
		responderErro(w, http.StatusNotFound, "Não achei essa conversa.")
		return
	}
	msg, err := a.enviarMidia(chat, dados, cab("X-Nome"), cab("X-Mime"), cab("X-Legenda"), cab("X-Tipo") == "voz")
	if err != nil {
		log.Printf("PAINEL: arquivo não enviado: %v", err)
		responderErro(w, http.StatusConflict, "Não consegui enviar o arquivo. Confira se o WhatsApp está conectado.")
		return
	}
	a.guardarRespostaDaEquipe(id, msg, nomeDe(r))
	log.Printf("PAINEL: %s enviado por uma pessoa para %s", msg.Tipo, fimDoNumero(id))
	responderOK(w, nil)
}

// entregarArquivo manda um arquivo guardado, aceitando pedidos de pedaco (para o
// navegador conseguir avancar e voltar em audio e video).
func entregarArquivo(w http.ResponseWriter, r *http.Request, caminho, tipo string, baixarCom string) {
	f, err := os.Open(caminho)
	if err != nil {
		http.Error(w, "arquivo não encontrado", http.StatusNotFound)
		return
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil || st.IsDir() {
		http.Error(w, "arquivo não encontrado", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", tipo)
	w.Header().Set("Cache-Control", "private, max-age=86400")
	// O arquivo veio de fora: nada dentro dele pode rodar script nem carregar coisa de fora.
	w.Header().Set("Content-Security-Policy", "default-src 'none'; img-src 'self' data:; media-src 'self'; style-src 'unsafe-inline'; object-src 'self'; frame-ancestors 'none'")
	if baixarCom != "" {
		w.Header().Set("Content-Disposition", "attachment; filename*=UTF-8''"+url.PathEscape(baixarCom))
	}
	http.ServeContent(w, r, "", st.ModTime(), f)
}

func (a *App) rotaMidia(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	caminho, ok := caminhoMidia(id)
	if !ok {
		http.Error(w, "arquivo não encontrado", http.StatusNotFound)
		return
	}
	ext := id[strings.LastIndexByte(id, '.'):]
	if tipo, inline := tiposInline[ext]; inline {
		entregarArquivo(w, r, caminho, tipo, "")
		return
	}
	entregarArquivo(w, r, caminho, "application/octet-stream", "arquivo"+ext)
}

func (a *App) rotaFoto(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	a.mu.Lock()
	l := a.leads[id]
	tem := l != nil && l.FotoID != ""
	a.mu.Unlock()
	if !tem {
		http.Error(w, "sem foto", http.StatusNotFound)
		return
	}
	entregarArquivo(w, r, arquivoFoto(id), "image/jpeg", "")
}

func (a *App) rotaEtiquetas(w http.ResponseWriter, r *http.Request) {
	var p struct {
		ID        string   `json:"id"`
		Etiquetas []string `json:"etiquetas"`
	}
	if !lerPedido(w, r, &p) {
		return
	}
	l := a.leadDoPedido(w, p.ID)
	if l == nil {
		return
	}
	antes := l.Etiquetas
	// Sem etiqueta repetida no painel: se já existe uma igual (sem contar maiúsculas e
	// acentos) em outro contato, vale a grafia que já existe.
	emUso := a.etiquetasEmUso()
	novas := limparEtiquetas(p.Etiquetas)
	for i, e := range novas {
		for _, existente := range emUso {
			if chaveTexto(existente) == chaveTexto(e) {
				novas[i] = existente
				break
			}
		}
	}
	l.Etiquetas = limparEtiquetas(novas)
	l.etiquetasEditadas(antes)
	if postas, tiradas := diferencaDeEtiquetas(antes, l.Etiquetas); len(postas)+len(tiradas) > 0 {
		l.evento("etiqueta", fraseDeEtiquetas(ouPadrao(nomeDe(r), "a equipe"), postas, tiradas))
	}
	a.unificarEtiquetas()
	etiquetas := append([]string{}, l.Etiquetas...)
	daIA := append([]string{}, l.EtiquetasIA...)
	a.salvarLeads()
	a.mu.Unlock()
	responderOK(w, map[string]any{"etiquetas": etiquetas, "etiquetas_ia": daIA})
}

// rotaCatalogoDeEtiquetas lista as etiquetas do painel, cada uma uma vez só.
func (a *App) rotaCatalogoDeEtiquetas(w http.ResponseWriter, r *http.Request) {
	a.mu.Lock()
	lista := a.catalogoDeEtiquetas()
	a.mu.Unlock()
	responderJSON(w, http.StatusOK, map[string]any{"etiquetas": lista})
}

// rotaRenomearEtiqueta troca o nome de uma etiqueta em todos os contatos. Se o nome novo
// já existir, as duas viram uma só.
func (a *App) rotaRenomearEtiqueta(w http.ResponseWriter, r *http.Request) {
	var p struct {
		De   string `json:"de"`
		Para string `json:"para"`
	}
	if !lerPedido(w, r, &p) {
		return
	}
	de := strings.TrimSpace(p.De)
	limpas := limparEtiquetas([]string{p.Para})
	if de == "" || len(limpas) == 0 {
		responderErro(w, http.StatusBadRequest, "Escreva o nome novo da etiqueta.")
		return
	}
	para := limpas[0]
	a.mu.Lock()
	// Se o nome novo já existe com outra grafia, fica a grafia que já existe.
	for _, existente := range a.etiquetasEmUso() {
		if chaveTexto(existente) == chaveTexto(para) && chaveTexto(existente) != chaveTexto(de) {
			para = existente
			break
		}
	}
	mudaram := a.renomearEtiqueta(de, para)
	if mudaram > 0 {
		a.unificarEtiquetas()
		a.salvarLeads()
	}
	lista := a.catalogoDeEtiquetas()
	a.mu.Unlock()
	if mudaram == 0 {
		responderErro(w, http.StatusNotFound, "Não achei essa etiqueta. Atualize a página e tente de novo.")
		return
	}
	a.registrar(usuarioDe(r), "etiqueta_renomeada", fmt.Sprintf("%s virou %s em %d contatos", de, para, mudaram))
	responderOK(w, map[string]any{"etiquetas": lista, "nome": para, "contatos": mudaram})
}

// rotaExcluirEtiqueta tira uma etiqueta de todos os contatos.
func (a *App) rotaExcluirEtiqueta(w http.ResponseWriter, r *http.Request) {
	var p struct {
		Nome string `json:"nome"`
	}
	if !lerPedido(w, r, &p) {
		return
	}
	nome := strings.TrimSpace(p.Nome)
	if nome == "" {
		responderErro(w, http.StatusBadRequest, "Diga qual etiqueta apagar.")
		return
	}
	a.mu.Lock()
	mudaram := a.renomearEtiqueta(nome, "")
	if mudaram > 0 {
		a.salvarLeads()
	}
	lista := a.catalogoDeEtiquetas()
	a.mu.Unlock()
	if mudaram == 0 {
		responderErro(w, http.StatusNotFound, "Não achei essa etiqueta. Atualize a página e tente de novo.")
		return
	}
	a.registrar(usuarioDe(r), "etiqueta_apagada", fmt.Sprintf("%s saiu de %d contatos", nome, mudaram))
	responderOK(w, map[string]any{"etiquetas": lista, "contatos": mudaram})
}

func (a *App) rotaRelatorio(w http.ResponseWriter, r *http.Request) {
	dias, err := strconv.Atoi(r.URL.Query().Get("dias"))
	if err != nil || (dias != 0 && dias != 1 && dias != 7 && dias != 30) {
		dias = 7
	}
	a.mu.Lock()
	rel := montarRelatorio(a.leadsOrdenados(), a.agenda, dias, time.Now())
	a.mu.Unlock()
	responderJSON(w, http.StatusOK, rel)
}

func (a *App) rotaExcluirLead(w http.ResponseWriter, r *http.Request) {
	var p pedidoID
	if !lerPedido(w, r, &p) {
		return
	}
	l := a.leadDoPedido(w, p.ID)
	if l == nil {
		return
	}
	contato := l.exibir()
	apagarMidias(l)
	delete(a.leads, p.ID)
	delete(a.contatos, p.ID)
	a.salvarLeads()
	a.mu.Unlock()
	log.Printf("PAINEL: conversa apagada")
	a.registrar(usuarioDe(r), "conversa_apagada", contato)
	responderOK(w, nil)
}

// celulaSegura evita que o Excel trate um texto vindo do WhatsApp como formula.
func celulaSegura(s string) string {
	s = strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(s, "\r", " "), "\n", " "))
	if s != "" && strings.ContainsRune("=+-@\t", rune(s[0])) {
		return "'" + s
	}
	return s
}

func dataBR(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Local().Format("02/01/2006 15:04")
}

// planilha devolve um CSV em UTF-8 com BOM e ponto e virgula, que e como o Excel
// em portugues abre direito.
func planilha(w http.ResponseWriter, nome string, linhas [][]string) {
	var buf bytes.Buffer
	buf.Write([]byte{0xEF, 0xBB, 0xBF})
	cw := csv.NewWriter(&buf)
	cw.Comma = ';'
	cw.UseCRLF = true
	for _, l := range linhas {
		_ = cw.Write(l)
	}
	cw.Flush()
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="`+nome+`"`)
	_, _ = w.Write(buf.Bytes())
}

func (a *App) rotaLeadsCSV(w http.ResponseWriter, r *http.Request) {
	linhas := [][]string{{"Nome", "Telefone", "Temperatura", "Pontuação", "Situação", "Etiquetas", "Resumo", "O que precisa", "Orçamento", "Decisor", "Prazo", "Etapa", "Motivo do transbordo", "Primeira mensagem", "Última mensagem"}}
	f := lerFiltroContatos(r)
	a.mu.Lock()
	for _, l := range a.contatosFiltrados(f) {
		linhas = append(linhas, []string{
			celulaSegura(ouPadrao(l.Nome, l.NomeWhats)),
			telefoneBonito(l.Telefone),
			nomesTemperatura[ouPadrao(l.Classificacao, "indefinido")],
			strconv.Itoa(l.Pontuacao),
			nomesSituacao[situacaoDe(l)],
			celulaSegura(strings.Join(l.Etiquetas, ", ")),
			celulaSegura(l.Resumo),
			celulaSegura(l.Necessidade),
			celulaSegura(l.Orcamento),
			celulaSegura(l.Decisor),
			celulaSegura(l.Prazo),
			celulaSegura(l.Etapa),
			celulaSegura(l.Motivo),
			dataBR(l.CriadoEm),
			dataBR(l.UltimaMsg),
		})
	}
	a.mu.Unlock()
	a.registrar(usuarioDe(r), "contatos_baixados", fmt.Sprintf("Relatório completo, %d contatos", len(linhas)-1))
	planilha(w, "leads.csv", linhas)
}

// ---------- dados do negocio e textos ----------

func (c Config) clonar() Config {
	c.Extras = append([]Variavel(nil), c.Extras...)
	c.NuncaResponder = append(listaNumeros(nil), c.NuncaResponder...)
	c.NumerosHumanos = append(listaNumeros(nil), c.NumerosHumanos...)
	c.Agenda.Servicos = append([]Servico(nil), c.Agenda.Servicos...)
	c.Agenda.Dias = append([]Expediente(nil), c.Agenda.Dias...)
	return c
}

func (a *App) rotaConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		a.mu.Lock()
		cfg := a.cfg.clonar()
		a.mu.Unlock()
		responderJSON(w, http.StatusOK, map[string]any{
			"config":    cfg,
			"variaveis": variaveis(cfg),
			"modelos":   modelosDisponiveis,
			"tons":      tonsDisponiveis,
		})
		return
	}
	corpo, err := io.ReadAll(r.Body)
	if r.Method != http.MethodPost || err != nil {
		responderErro(w, http.StatusBadRequest, "Não entendi os dados enviados. Tente de novo.")
		return
	}

	a.mu.Lock()
	nova := a.cfg.clonar()
	// O que nao vier no pedido fica como estava.
	if err := json.Unmarshal(corpo, &nova); err != nil {
		a.mu.Unlock()
		log.Printf("PAINEL: dados recusados: %v", err)
		responderErro(w, http.StatusBadRequest, "Algum campo veio em um formato que não entendi. Confira os números e tente de novo.")
		return
	}
	nova.Salvo, nova.Ligado = a.cfg.Salvo, a.cfg.Ligado
	// Quem responde (Claude ou ChatGPT) so muda pelas rotas de conexao da IA.
	nova.Provedor, nova.ModeloOpenAI = a.cfg.Provedor, a.cfg.ModeloOpenAI
	// Quem recebe os avisos: vale a lista nova; o campo antigo, de um numero so, ainda e aceito.
	var chaves map[string]json.RawMessage
	_ = json.Unmarshal(corpo, &chaves)
	candidatos := []string(nova.NumerosHumanos)
	if _, veioLista := chaves["numeros_humanos"]; veioLista {
		nova.NumeroHumano = ""
	} else if _, veioUm := chaves["numero_humano"]; veioUm {
		nova.NumerosHumanos = nil
		candidatos = strings.FieldsFunc(nova.NumeroHumano, func(r rune) bool { return r == ',' || r == ';' || r == '\n' })
	}
	for _, n := range candidatos {
		if d := soDigitos(n); d != "" && (len(d) < 10 || len(d) > 15) {
			a.mu.Unlock()
			responderErro(w, http.StatusBadRequest, "O número "+cortar(n, 20)+" não parece completo. Use DDI e DDD, só com números. Exemplo: 5527999999999")
			return
		}
	}
	nova.normalizar()
	// Os dias da agenda são a fonte do horário de funcionamento: quando as regras da
	// agenda chegam no pedido, o texto que o assistente informa é escrito a partir delas.
	if _, veioAgenda := chaves["agenda"]; veioAgenda {
		if texto := nova.Agenda.horarioPorExtenso(); texto != "" {
			nova.Horario = texto
		}
	}
	if nova.Negocio == "" && a.cfg.Salvo {
		a.mu.Unlock()
		responderErro(w, http.StatusBadRequest, "Preencha o nome do negócio.")
		return
	}
	if nova.Negocio != "" {
		nova.Salvo = true
	}
	a.cfg = nova
	a.salvarConfig()
	cfg := a.cfg.clonar()
	a.mu.Unlock()

	log.Printf("PAINEL: dados salvos")
	a.registrar(usuarioDe(r), "dados_salvos", "")
	responderOK(w, map[string]any{"mensagem": "Salvo", "config": cfg})
}

func (a *App) rotaTextos(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		resp := map[string]any{}
		for _, t := range textosEditaveis {
			texto, padrao := lerTexto(t)
			resp[t.Chave] = map[string]any{"texto": texto, "padrao": padrao}
		}
		responderJSON(w, http.StatusOK, resp)
		return
	}
	var p struct {
		Arquivo string `json:"arquivo"`
		Texto   string `json:"texto"`
	}
	if !lerPedido(w, r, &p) {
		return
	}
	t, ok := textoPorChave(p.Arquivo)
	if !ok {
		responderErro(w, http.StatusBadRequest, "Não conheço esse texto.")
		return
	}
	if strings.TrimSpace(p.Texto) == "" && t.Modelo != "" {
		responderErro(w, http.StatusBadRequest, "O texto não pode ficar vazio. Se quiser recomeçar, use Voltar ao modelo original.")
		return
	}
	if err := salvarTexto(t, p.Texto); err != nil {
		responderErro(w, http.StatusBadRequest, "Não consegui salvar: "+err.Error()+".")
		return
	}
	log.Printf("PAINEL: texto %s salvo", t.Chave)
	a.registrar(usuarioDe(r), "texto_alterado", t.Chave)
	responderOK(w, map[string]any{"mensagem": "Salvo"})
}

func (a *App) rotaRestaurarTexto(w http.ResponseWriter, r *http.Request) {
	var p struct {
		Arquivo string `json:"arquivo"`
	}
	if !lerPedido(w, r, &p) {
		return
	}
	t, ok := textoPorChave(p.Arquivo)
	if !ok {
		responderErro(w, http.StatusBadRequest, "Não conheço esse texto.")
		return
	}
	if err := salvarTexto(t, t.Modelo); err != nil {
		responderErro(w, http.StatusInternalServerError, "Não consegui voltar ao modelo original.")
		return
	}
	log.Printf("PAINEL: texto %s voltou ao modelo original", t.Chave)
	a.registrar(usuarioDe(r), "texto_alterado", t.Chave+" (voltou ao modelo original)")
	responderOK(w, map[string]any{"texto": t.Modelo})
}

// ---------- ajuda da IA e treino ----------

func (a *App) rotaIANegocio(w http.ResponseWriter, r *http.Request) {
	var p struct {
		Descricao string `json:"descricao"`
	}
	if !lerPedido(w, r, &p) {
		return
	}
	if len(strings.TrimSpace(p.Descricao)) < 10 {
		responderErro(w, http.StatusBadRequest, "Conte um pouco mais sobre o negócio para a IA conseguir preencher.")
		return
	}
	campos, agenda, err := a.preencherNegocio(p.Descricao)
	if err != nil {
		erroDaIA(w, err)
		return
	}
	// agenda vem nula quando a descrição não fala de horário nem de serviço com hora marcada.
	responderOK(w, map[string]any{"campos": campos, "agenda": agenda})
}

func (a *App) rotaIAMelhorar(w http.ResponseWriter, r *http.Request) {
	var p struct {
		Arquivo string `json:"arquivo"`
		Texto   string `json:"texto"`
		Pedido  string `json:"pedido"`
	}
	if !lerPedido(w, r, &p) {
		return
	}
	if _, ok := textoPorChave(p.Arquivo); !ok {
		responderErro(w, http.StatusBadRequest, "Não conheço esse texto.")
		return
	}
	if strings.TrimSpace(p.Pedido) == "" {
		responderErro(w, http.StatusBadRequest, "Escreva o que você quer mudar.")
		return
	}
	novo, mudancas, err := a.melhorarTexto(p.Arquivo, p.Texto, p.Pedido)
	if err != nil {
		erroDaIA(w, err)
		return
	}
	responderOK(w, map[string]any{"texto": novo, "mudancas": mudancas})
}

func (a *App) rotaIACerebro(w http.ResponseWriter, r *http.Request) {
	var p struct{}
	if !lerPedido(w, r, &p) {
		return
	}
	texto, err := a.organizarCerebro()
	if err != nil {
		erroDaIA(w, err)
		return
	}
	responderOK(w, map[string]any{"texto": texto})
}

func (a *App) rotaTreinar(w http.ResponseWriter, r *http.Request) {
	var p struct {
		Mensagens []struct {
			De    string `json:"de"`
			Texto string `json:"texto"`
		} `json:"mensagens"`
		Lead LeadIA `json:"lead"`
	}
	if !lerPedido(w, r, &p) {
		return
	}
	n := len(p.Mensagens)
	if n == 0 || p.Mensagens[n-1].De != "cliente" || strings.TrimSpace(p.Mensagens[n-1].Texto) == "" {
		responderErro(w, http.StatusBadRequest, "Escreva uma mensagem como se fosse o cliente.")
		return
	}
	var anteriores []Msg
	for _, m := range p.Mensagens[:n-1] {
		de := "cliente"
		if m.De == "atendente" || m.De == "humano" {
			de = m.De
		}
		anteriores = append(anteriores, Msg{De: de, Texto: cortar(m.Texto, maxTextoMsg)})
	}
	nova := cortar(strings.TrimSpace(p.Mensagens[n-1].Texto), maxTextoMsg)
	l := &Lead{
		Nome: p.Lead.Nome, Necessidade: p.Lead.Necessidade, Orcamento: p.Lead.Orcamento,
		Decisor: p.Lead.Decisor, Prazo: p.Lead.Prazo, Pontuacao: limitar(int(p.Lead.Pontuacao), 0, 100),
		Classificacao: ouPadrao(p.Lead.Classificacao, "indefinido"), Etapa: p.Lead.Etapa, Resumo: p.Lead.Resumo,
		// No simulador, todas as etiquetas contam como postas pela IA, para ela poder trocar.
		Etiquetas: limparEtiquetas(p.Lead.Etiquetas), EtiquetasIA: limparEtiquetas(p.Lead.Etiquetas),
	}

	a.mu.Lock()
	cfg := a.cfg.clonar()
	emUso := a.etiquetasEmUso()
	a.mu.Unlock()

	sistema, trechos := a.montarSistema(cfg, consultaDaConversa(anteriores, []string{nova}), "")
	entrada := montarEntrada(anteriores, []string{nova}, l, emUso)

	a.vagas <- struct{}{}
	inicio := time.Now()
	saida, modo, err := a.cerebro.Responder(modeloDe(cfg), sistema, entrada, nova)
	<-a.vagas
	if err != nil {
		erroDaIA(w, err)
		return
	}
	aplicarSaida(l, saida)
	l.etiquetasDaIA(saida.Lead.Etiquetas, emUso)
	if trechos == nil {
		trechos = []Trecho{}
	}
	responderOK(w, map[string]any{
		"resposta":    saida.Resposta,
		"transbordo":  saida.Transbordo || (l.Classificacao == "quente" && !cfg.Agenda.Ativa),
		"motivo":      saida.Motivo,
		"lead":        leadParaIA(l),
		"etiquetas":   append([]string{}, l.Etiquetas...),
		"agendamento": saida.Agendamento,
		"trechos":     trechos,
		"modo":        modo,
		"ms":          time.Since(inicio).Milliseconds(),
	})
}

func (a *App) rotaAprendizado(w http.ResponseWriter, r *http.Request) {
	var p struct {
		Situacao string `json:"situacao"`
		Resposta string `json:"resposta"`
	}
	if !lerPedido(w, r, &p) {
		return
	}
	situacao := cortar(umaLinha(p.Situacao), 500)
	resposta := cortar(umaLinha(p.Resposta), 1200)
	if resposta == "" {
		responderErro(w, http.StatusBadRequest, "Escreva como o assistente deveria ter respondido.")
		return
	}
	t, _ := textoPorChave("aprendizados")
	atual, _ := lerTexto(t)
	linha := "- Responda nesta linha: \"" + resposta + "\""
	if situacao != "" {
		linha = "- Quando o cliente disser algo como \"" + situacao + "\", responda nesta linha: \"" + resposta + "\""
	}
	novo := strings.TrimSpace(strings.TrimSpace(atual)+"\n"+linha) + "\n"
	if err := salvarTexto(t, novo); err != nil {
		responderErro(w, http.StatusBadRequest, "Não consegui guardar o aprendizado: "+err.Error()+".")
		return
	}
	log.Printf("PAINEL: aprendizado guardado")
	responderOK(w, map[string]any{"texto": novo})
}

// ---------- base de conhecimento ----------

type materialVisao struct {
	ID         string    `json:"id"`
	Nome       string    `json:"nome"`
	Tipo       string    `json:"tipo"`
	Tamanho    int       `json:"tamanho"`
	Titulo     string    `json:"titulo"`
	Resumo     string    `json:"resumo"`
	Perguntas  int       `json:"perguntas"`
	Trechos    int       `json:"trechos"`
	Caracteres int       `json:"caracteres"`
	Ativo      bool      `json:"ativo"`
	Estado     string    `json:"estado"`
	Erro       string    `json:"erro"`
	CriadoEm   time.Time `json:"criado_em"`
}

func visaoMaterial(m Material) materialVisao {
	return materialVisao{
		ID: m.ID, Nome: m.Nome, Tipo: m.Tipo, Tamanho: m.Tamanho, Titulo: m.Titulo, Resumo: m.Resumo,
		Perguntas: len(m.Perguntas), Trechos: len(m.Trechos), Caracteres: m.Caracteres,
		Ativo: m.Ativo, Estado: m.Estado, Erro: m.Erro, CriadoEm: m.CriadoEm,
	}
}

func (a *App) rotaConhecimento(w http.ResponseWriter, r *http.Request) {
	lista := a.base.lista()
	visoes := make([]materialVisao, 0, len(lista))
	for _, m := range lista {
		visoes = append(visoes, visaoMaterial(m))
	}
	responderJSON(w, http.StatusOK, map[string]any{
		"materiais": visoes,
		"formatos":  formatosAceitos,
		"limite_mb": maxArquivo >> 20,
	})
}

func (a *App) rotaEnviarMaterial(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		responderErro(w, http.StatusMethodNotAllowed, "Use o botão de enviar da página.")
		return
	}
	nome, err := url.QueryUnescape(r.Header.Get("X-Nome"))
	if err != nil || strings.TrimSpace(nome) == "" {
		nome = "arquivo"
	}
	dados, err := io.ReadAll(r.Body)
	if err != nil {
		responderErro(w, http.StatusRequestEntityTooLarge, fmt.Sprintf("Este arquivo passa de %d MB. Envie um arquivo menor.", maxArquivo>>20))
		return
	}
	if len(dados) == 0 {
		responderErro(w, http.StatusBadRequest, "O arquivo chegou vazio.")
		return
	}
	m, err := a.base.adicionar(nome, dados)
	if err != nil {
		msg := err.Error()
		responderErro(w, http.StatusBadRequest, strings.ToUpper(msg[:1])+msg[1:]+".")
		return
	}
	log.Printf("CONHECIMENTO: material novo (%s, %d trechos)", m.Tipo, len(m.Trechos))
	a.registrar(usuarioDe(r), "material_enviado", m.Nome)
	go a.organizarMaterial(m.ID)
	responderOK(w, map[string]any{"material": visaoMaterial(*m)})
}

func (a *App) rotaTextoMaterial(w http.ResponseWriter, r *http.Request) {
	var p struct {
		Titulo string `json:"titulo"`
		Texto  string `json:"texto"`
	}
	if !lerPedido(w, r, &p) {
		return
	}
	m, err := a.base.adicionarTexto(p.Titulo, p.Texto)
	if err != nil {
		responderErro(w, http.StatusBadRequest, "Cole um texto com pelo menos uma frase.")
		return
	}
	log.Printf("CONHECIMENTO: texto colado (%d trechos)", len(m.Trechos))
	go a.organizarMaterial(m.ID)
	responderOK(w, map[string]any{"material": visaoMaterial(*m)})
}

func (a *App) rotaAlterarMaterial(w http.ResponseWriter, r *http.Request) {
	var p struct {
		ID    string `json:"id"`
		Ativo *bool  `json:"ativo"`
	}
	if !lerPedido(w, r, &p) {
		return
	}
	m, ok := a.base.alterar(p.ID, func(m *Material) {
		if p.Ativo != nil {
			m.Ativo = *p.Ativo
		}
	})
	if !ok {
		responderErro(w, http.StatusNotFound, "Não achei esse material.")
		return
	}
	responderOK(w, map[string]any{"material": visaoMaterial(*m)})
}

func (a *App) rotaExcluirMaterial(w http.ResponseWriter, r *http.Request) {
	var p pedidoID
	if !lerPedido(w, r, &p) {
		return
	}
	if !a.base.excluir(p.ID) {
		responderErro(w, http.StatusNotFound, "Não achei esse material.")
		return
	}
	log.Printf("CONHECIMENTO: material apagado")
	a.registrar(usuarioDe(r), "material_removido", "")
	responderOK(w, nil)
}

// ---------- agenda ----------

type agendamentoVisao struct {
	Agendamento
	TelefoneBonito string `json:"telefone_bonito"`
}

func visaoAgendamento(ag *Agendamento) agendamentoVisao {
	return agendamentoVisao{Agendamento: *ag, TelefoneBonito: telefoneBonito(ag.Telefone)}
}

func (a *App) rotaAgenda(w http.ResponseWriter, r *http.Request) {
	agora := time.Now()
	a.mu.Lock()
	cfg := a.cfg.Agenda
	var futuros, passados []agendamentoVisao
	for _, ag := range a.agenda {
		switch {
		case ag.fim().After(agora):
			futuros = append(futuros, visaoAgendamento(ag))
		case ag.Inicio.After(agora.AddDate(0, 0, -30)):
			passados = append(passados, visaoAgendamento(ag))
		}
	}
	livres := cfg.livresProximosDias(a.agenda, cfg.menorDuracao(), agora)
	a.mu.Unlock()

	sort.Slice(futuros, func(i, j int) bool { return futuros[i].Inicio.Before(futuros[j].Inicio) })
	sort.Slice(passados, func(i, j int) bool { return passados[i].Inicio.After(passados[j].Inicio) })
	todos := append(futuros, passados...)
	if todos == nil {
		todos = []agendamentoVisao{}
	}
	if livres == nil {
		livres = []diaLivre{}
	}
	responderJSON(w, http.StatusOK, map[string]any{
		"ativa":        cfg.Ativa,
		"servicos":     cfg.Servicos,
		"agendamentos": todos,
		"livres":       livres,
	})
}

func (a *App) rotaMarcar(w http.ResponseWriter, r *http.Request) {
	var p struct {
		Nome     string `json:"nome"`
		Telefone string `json:"telefone"`
		Servico  string `json:"servico"`
		Data     string `json:"data"`
		Hora     string `json:"hora"`
		Obs      string `json:"obs"`
	}
	if !lerPedido(w, r, &p) {
		return
	}
	if strings.TrimSpace(p.Nome) == "" {
		responderErro(w, http.StatusBadRequest, "Escreva o nome de quem vai ser atendido.")
		return
	}
	agora := time.Now()
	inicio, err := interpretarDataHora(p.Data, p.Hora, agora)
	if err != nil {
		responderErro(w, http.StatusBadRequest, "Escolha o dia e a hora.")
		return
	}
	a.mu.Lock()
	ag, err := a.marcar(p.Nome, p.Telefone, "", p.Servico, inicio, "painel", p.Obs, agora)
	var v agendamentoVisao
	if err == nil {
		v = visaoAgendamento(ag)
	}
	a.mu.Unlock()
	if err != nil {
		msg := err.Error()
		responderErro(w, http.StatusConflict, "Não deu para marcar: "+msg+".")
		return
	}
	log.Printf("AGENDA: horário marcado pelo painel")
	responderOK(w, map[string]any{"agendamento": v})
}

func (a *App) rotaCancelar(w http.ResponseWriter, r *http.Request) {
	var p pedidoID
	if !lerPedido(w, r, &p) {
		return
	}
	a.mu.Lock()
	achou := false
	for _, ag := range a.agenda {
		if ag.ID == p.ID {
			achou = true
			ag.Status = "cancelado"
		}
	}
	if achou {
		a.salvarAgenda()
	}
	a.mu.Unlock()
	if !achou {
		responderErro(w, http.StatusNotFound, "Não achei esse horário.")
		return
	}
	log.Printf("AGENDA: horário cancelado pelo painel")
	responderOK(w, nil)
}

func (a *App) rotaAgendaCSV(w http.ResponseWriter, r *http.Request) {
	linhas := [][]string{{"Data", "Hora", "Nome", "Telefone", "Serviço", "Duração (min)", "Situação", "Marcado por", "Observação"}}
	a.mu.Lock()
	lista := append([]*Agendamento(nil), a.agenda...)
	sort.Slice(lista, func(i, j int) bool { return lista[i].Inicio.Before(lista[j].Inicio) })
	for _, ag := range lista {
		linhas = append(linhas, []string{
			ag.Inicio.Local().Format("02/01/2006"),
			ag.Inicio.Local().Format("15:04"),
			celulaSegura(ag.Nome),
			telefoneBonito(ag.Telefone),
			celulaSegura(ag.Servico),
			strconv.Itoa(ag.DuracaoMin),
			celulaSegura(ag.Status),
			celulaSegura(ag.Origem),
			celulaSegura(ag.Obs),
		})
	}
	a.mu.Unlock()
	planilha(w, "agenda.csv", linhas)
}
