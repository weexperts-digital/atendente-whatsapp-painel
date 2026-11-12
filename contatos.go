package main

import (
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
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
