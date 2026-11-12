package main

import (
	"fmt"
	"log"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// filtroContatos e o que a aba Contatos (e as planilhas) usam para escolher quem aparece.
type filtroContatos struct {
	Busca       string // nome, telefone, etiqueta ou trecho do que a pessoa precisa
	Situacao    string // assistente, fila ou atendimento
	Temperatura string // quente, morno, frio ou indefinido
	Etiqueta    string
	Ordem       string // recentes (padrao), antigos, nome ou pontuacao
}

func lerFiltroContatos(r *http.Request) filtroContatos {
	q := r.URL.Query()
	limpo := func(chave string, max int) string { return cortar(strings.TrimSpace(q.Get(chave)), max) }
	return filtroContatos{
		Busca:       limpo("q", 80),
		Situacao:    limpo("situacao", 20),
		Temperatura: limpo("temperatura", 20),
		Etiqueta:    limpo("etiqueta", 40),
		Ordem:       limpo("ordem", 20),
	}
}

// dobrar deixa o texto sem acento e em minusculas, para comparar.
func dobrar(s string) string {
	return strings.ToLower(semAcento(s))
}

// situacaoDe traduz o status guardado para as tres situacoes da tela.
func situacaoDe(l *Lead) string {
	switch l.Status {
	case statusHumano:
		return "fila"
	case statusAtendimento:
		return "atendimento"
	}
	return "assistente"
}

func (f filtroContatos) passa(l *Lead) bool {
	if f.Situacao != "" && situacaoDe(l) != f.Situacao {
		return false
	}
	if f.Temperatura != "" && ouPadrao(l.Classificacao, "indefinido") != f.Temperatura {
		return false
	}
	if f.Etiqueta != "" {
		tem := false
		for _, e := range l.Etiquetas {
			tem = tem || strings.EqualFold(e, f.Etiqueta)
		}
		if !tem {
			return false
		}
	}
	if f.Busca == "" {
		return true
	}
	// Quem digita so numeros esta procurando um telefone.
	if d := soDigitos(f.Busca); len(d) >= 3 && strings.Trim(f.Busca, "0123456789 +()-.") == "" {
		return strings.Contains(soDigitos(l.Telefone), d)
	}
	alvo := dobrar(strings.Join([]string{l.Nome, l.NomeWhats, telefoneBonito(l.Telefone), strings.Join(l.Etiquetas, " "), l.Necessidade, l.Resumo}, " "))
	for _, palavra := range strings.Fields(dobrar(f.Busca)) {
		if !strings.Contains(alvo, palavra) {
			return false
		}
	}
	return true
}

// contatosFiltrados devolve os contatos que passam no filtro, ja na ordem pedida.
// Chame com a.mu travado.
func (a *App) contatosFiltrados(f filtroContatos) []*Lead {
	lista := make([]*Lead, 0, len(a.leads))
	for _, l := range a.leadsOrdenados() { // ja vem do mais recente para o mais antigo
		if f.passa(l) {
			lista = append(lista, l)
		}
	}
	switch f.Ordem {
	case "antigos":
		sort.SliceStable(lista, func(i, j int) bool { return lista[i].CriadoEm.Before(lista[j].CriadoEm) })
	case "nome":
		sort.SliceStable(lista, func(i, j int) bool { return dobrar(lista[i].exibir()) < dobrar(lista[j].exibir()) })
	case "pontuacao":
		sort.SliceStable(lista, func(i, j int) bool { return lista[i].Pontuacao > lista[j].Pontuacao })
	}
	return lista
}

// totalMensagens conta as mensagens trocadas com a pessoa, sem as linhas internas.
func (l *Lead) totalMensagens() int {
	if l.TotalMsgs > 0 {
		return l.TotalMsgs
	}
	n := 0
	for _, m := range l.Historico {
		if m.De != "sistema" {
			n++
		}
	}
	return n
}

// rotaContatos lista todo mundo que ja escreveu para o negocio.
func (a *App) rotaContatos(w http.ResponseWriter, r *http.Request) {
	f := lerFiltroContatos(r)
	agora := time.Now()
	a.mu.Lock()
	filtrados := a.contatosFiltrados(f)
	contatos := make([]leadVisao, 0, len(filtrados))
	for _, l := range filtrados {
		v := a.visaoLead(l, agora)
		v.TotalMsgs = l.totalMensagens()
		contatos = append(contatos, v)
	}
	resp := map[string]any{
		"contatos":    contatos,
		"total":       len(contatos),
		"total_geral": len(a.leads),
		"etiquetas":   a.etiquetasEmUso(),
	}
	a.mu.Unlock()
	responderJSON(w, http.StatusOK, resp)
}

var nomesSituacao = map[string]string{
	"assistente":  "Com o assistente",
	"fila":        "Na fila",
	"atendimento": "Em atendimento",
}

var nomesTemperatura = map[string]string{
	"quente": "Quente", "morno": "Morno", "frio": "Frio", "indefinido": "Sem classificação",
}

// rotaContatosCSV entrega a planilha simples de contatos, com o mesmo filtro da tela.
func (a *App) rotaContatosCSV(w http.ResponseWriter, r *http.Request) {
	f := lerFiltroContatos(r)
	agora := time.Now()
	linhas := [][]string{{"Nome", "Nome no WhatsApp", "Telefone", "Temperatura", "Pontuação", "Situação", "Etiquetas", "Primeiro contato", "Última mensagem", "Mensagens", "Próximo agendamento"}}
	a.mu.Lock()
	for _, l := range a.contatosFiltrados(f) {
		proximo := ""
		if futuros := a.futurosDoLead(l.ID, agora); len(futuros) > 0 {
			proximo = strings.TrimSpace(futuros[0].Servico + " " + dataBR(futuros[0].Inicio))
		}
		linhas = append(linhas, []string{
			celulaSegura(l.Nome),
			celulaSegura(l.NomeWhats),
			telefoneBonito(l.Telefone),
			nomesTemperatura[ouPadrao(l.Classificacao, "indefinido")],
			strconv.Itoa(l.Pontuacao),
			nomesSituacao[situacaoDe(l)],
			celulaSegura(strings.Join(l.Etiquetas, ", ")),
			dataBR(l.CriadoEm),
			dataBR(l.UltimaMsg),
			strconv.Itoa(l.totalMensagens()),
			celulaSegura(proximo),
		})
	}
	a.mu.Unlock()
	a.registrar(usuarioDe(r), "contatos_baixados", fmt.Sprintf("Lista de contatos, %d contatos", len(linhas)-1))
	planilha(w, "contatos.csv", linhas)
}

// ---------- ficha do contato ----------

const (
	maxAnotacoes    = 4000 // letras das anotacoes da equipe sobre um contato
	maxEventosFicha = 500  // quantos registros a linha do tempo da ficha devolve de uma vez
)

// rotasDaFicha registra as rotas da ficha do contato (dados, historico e anotacoes).
func (a *App) rotasDaFicha(mux *http.ServeMux) {
	mux.HandleFunc("/api/contato", a.rotaContato)
	mux.HandleFunc("/api/contato/anotacoes", a.rotaAnotacoes)
}

// contatoVisao e o contato da ficha: os campos do lead (com a qualificacao que a IA
// preenche) mais as anotacoes da equipe.
type contatoVisao struct {
	leadVisao
	Anotacoes string `json:"anotacoes"`
}

// eventoVisao e uma linha da linha do tempo do contato. O cliente nunca ve isso.
type eventoVisao struct {
	N      int64     `json:"n"` // numero da mensagem no banco (0 = ainda nao gravada, ou "primeiro_contato")
	Hora   time.Time `json:"hora"`
	Evento string    `json:"evento"`
	Texto  string    `json:"texto"`
	Autor  string    `json:"autor"`
}

// eventosDoContato le do banco as linhas de historico interno (de = 'sistema') de um
// contato, da mais nova para a mais antiga. A memoria guarda so as ultimas mensagens
// de cada conversa; o banco guarda todas.
func (b *Banco) eventosDoContato(contato string, limite int) ([]Msg, bool, error) {
	linhas, err := b.db.Query(`SELECT `+colunasMsg+` FROM mensagens WHERE contato_id = ? AND de = 'sistema' ORDER BY n DESC LIMIT ?`, contato, limite+1)
	if err != nil {
		return nil, false, err
	}
	defer linhas.Close()
	var msgs []Msg
	for linhas.Next() {
		m, err := lerMsg(linhas.Scan)
		if err != nil {
			return nil, false, err
		}
		msgs = append(msgs, m)
	}
	temMais := len(msgs) > limite
	if temMais {
		msgs = msgs[:limite]
	}
	return msgs, temMais, linhas.Err()
}

// agendamentoDoContato diz se o horario e desta pessoa. Os marcados pelo assistente
// trazem o id da conversa; os marcados pela equipe no painel so tem o telefone.
func agendamentoDoContato(ag *Agendamento, l *Lead) bool {
	if ag.LeadID != "" {
		return ag.LeadID == l.ID
	}
	return mesmoNumero(ag.Telefone, l.Telefone)
}

// chaveEvento reconhece o mesmo registro vindo da memoria e do banco.
func chaveEvento(m *Msg) string {
	return horaExata(m.Hora) + "\x1f" + m.Evento + "\x1f" + m.Texto
}

// linhaDoTempo junta os registros do banco com os que ainda estao so na memoria e
// devolve tudo do mais novo para o mais antigo, terminando no primeiro contato.
func linhaDoTempo(doBanco, soltos []Msg, criadoEm time.Time, limite int) ([]eventoVisao, bool) {
	vistos := make(map[string]bool, len(doBanco))
	for i := range doBanco {
		vistos[chaveEvento(&doBanco[i])] = true
	}
	// Os soltos sao os mais novos: entram na frente, do ultimo para o primeiro.
	todos := make([]Msg, 0, len(doBanco)+len(soltos))
	for i := len(soltos) - 1; i >= 0; i-- {
		if !vistos[chaveEvento(&soltos[i])] {
			todos = append(todos, soltos[i])
		}
	}
	todos = append(todos, doBanco...)
	sort.SliceStable(todos, func(i, j int) bool { return todos[i].Hora.After(todos[j].Hora) })
	cortou := len(todos) > limite
	if cortou {
		todos = todos[:limite]
	}
	linha := make([]eventoVisao, 0, len(todos)+1)
	for _, m := range todos {
		linha = append(linha, eventoVisao{N: m.N, Hora: m.Hora, Evento: m.Evento, Texto: m.Texto, Autor: m.Autor})
	}
	if !criadoEm.IsZero() {
		linha = append(linha, eventoVisao{Hora: criadoEm, Evento: "primeiro_contato", Texto: "Primeiro contato pelo WhatsApp"})
	}
	return linha, cortou
}

// rotaContato entrega a ficha de uma pessoa: os dados, os agendamentos (futuros,
// passados e cancelados) e a linha do tempo, que vem do banco.
func (a *App) rotaContato(w http.ResponseWriter, r *http.Request) {
	agora := time.Now()
	id := r.URL.Query().Get("id")
	a.mu.Lock()
	l := a.leads[id]
	if l == nil {
		a.mu.Unlock()
		responderErro(w, http.StatusNotFound, "Não achei esse contato. Ele pode ter sido apagado. Volte para a lista de contatos.")
		return
	}
	v := contatoVisao{leadVisao: a.visaoLead(l, agora), Anotacoes: l.Anotacoes}
	ags := []agendamentoVisao{}
	var proximo *Agendamento
	for _, ag := range a.agenda {
		if !agendamentoDoContato(ag, l) {
			continue
		}
		ags = append(ags, visaoAgendamento(ag))
		if ag.Status == "marcado" && ag.Inicio.After(agora) && (proximo == nil || ag.Inicio.Before(proximo.Inicio)) {
			proximo = ag
		}
	}
	// O proximo horario sai da mesma lista que a ficha mostra.
	v.ProximoAgendado = nil
	if proximo != nil {
		v.ProximoAgendado = &agendamentoCurto{Servico: proximo.Servico, Inicio: proximo.Inicio}
	}
	// Registros que ainda nao foram para o banco (sem banco, nos testes, sao todos).
	var soltos []Msg
	for _, m := range l.Historico {
		if m.De == "sistema" && m.N == 0 {
			soltos = append(soltos, m)
		}
	}
	criadoEm := l.CriadoEm
	a.mu.Unlock()

	var doBanco []Msg
	temMais := false
	if a.banco != nil {
		var err error
		doBanco, temMais, err = a.banco.eventosDoContato(id, maxEventosFicha)
		if err != nil {
			log.Printf("PAINEL: não li o histórico do contato: %v", err)
			responderErro(w, http.StatusInternalServerError, "Não consegui buscar o histórico deste contato. Tente de novo em alguns segundos.")
			return
		}
	}
	historico, cortou := linhaDoTempo(doBanco, soltos, criadoEm, maxEventosFicha)
	sort.SliceStable(ags, func(i, j int) bool { return ags[i].Inicio.After(ags[j].Inicio) })

	// Ao abrir a ficha, confere a foto do contato (no maximo uma vez por dia).
	if cli := a.cliente(); cli != nil && cli.IsConnected() && cli.IsLoggedIn() {
		go a.atualizarFoto(cli, id)
	}
	responderOK(w, map[string]any{
		"contato":            v,
		"agendamentos":       ags,
		"historico":          historico,
		"historico_tem_mais": temMais || cortou,
	})
}

// limparAnotacoes deixa o texto com quebras de linha simples, sem caracteres de
// controle e sem espaco sobrando no comeco e no fim.
func limparAnotacoes(s string) string {
	s = strings.ReplaceAll(strings.ReplaceAll(s, "\r\n", "\n"), "\r", "\n")
	s = strings.Map(func(r rune) rune {
		if r != '\n' && r != '\t' && unicode.IsControl(r) {
			return -1
		}
		return r
	}, s)
	return strings.TrimSpace(s)
}

// rotaAnotacoes grava o texto livre que a equipe escreve sobre o contato. Fica so no
// painel: nao vai para o cliente.
func (a *App) rotaAnotacoes(w http.ResponseWriter, r *http.Request) {
	var p struct {
		ID        string `json:"id"`
		Anotacoes string `json:"anotacoes"`
	}
	if !lerPedido(w, r, &p) {
		return
	}
	texto := limparAnotacoes(p.Anotacoes)
	if n := utf8.RuneCountInString(texto); n > maxAnotacoes {
		responderErro(w, http.StatusBadRequest, fmt.Sprintf("As anotações têm %d letras e o limite é %d. Encurte o texto e salve de novo.", n, maxAnotacoes))
		return
	}
	a.mu.Lock()
	l := a.leads[p.ID]
	if l == nil {
		a.mu.Unlock()
		responderErro(w, http.StatusNotFound, "Não achei esse contato. Ele pode ter sido apagado. Volte para a lista de contatos.")
		return
	}
	l.Anotacoes = texto
	a.salvarLeads()
	a.mu.Unlock()
	log.Printf("PAINEL: anotações do contato salvas")
	responderOK(w, map[string]any{"anotacoes": texto})
}
