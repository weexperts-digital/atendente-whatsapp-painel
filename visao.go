package main

import (
	"net/http"
	"sort"
	"time"
)

// Pagina Inicio: a visao geral do painel. Junta em um pedido so o que a pessoa precisa
// ver ao abrir: se esta tudo funcionando, os numeros de hoje e da semana, quem esta
// esperando uma pessoa, os proximos horarios e as ultimas conversas.

// pecaDaVisao e uma das partes que precisam estar de pe para o atendimento funcionar.
type pecaDaVisao struct {
	Chave   string `json:"chave"`   // atendente, whatsapp, ia ou agenda
	Certo   bool   `json:"certo"`   // esta funcionando
	Titulo  string `json:"titulo"`  // frase curta do estado
	Detalhe string `json:"detalhe"` // o que isso quer dizer, ou o que fazer
}

type esperaDaVisao struct {
	ID     string    `json:"id"`
	Exibir string    `json:"exibir"`
	Motivo string    `json:"motivo"`
	Desde  time.Time `json:"desde"`
	Classe string    `json:"classificacao"`
}

type conversaDaVisao struct {
	ID        string    `json:"id"`
	Exibir    string    `json:"exibir"`
	Previa    string    `json:"previa"`
	PreviaDe  string    `json:"previa_de"`
	UltimaMsg time.Time `json:"ultima_msg"`
	Classe    string    `json:"classificacao"`
	Situacao  string    `json:"situacao"`
}

// ultimaDe acha a hora da mensagem mais recente de um lado da conversa.
func ultimaDe(leads []*Lead, de string) time.Time {
	var ultima time.Time
	for _, l := range leads {
		for i := len(l.Historico) - 1; i >= 0; i-- {
			if m := l.Historico[i]; m.De == de {
				if m.Hora.After(ultima) {
					ultima = m.Hora
				}
				break
			}
		}
	}
	return ultima
}

// pecasDaVisao diz, em palavras simples, se cada parte do atendimento esta de pe.
// Chame com a.mu travado.
func (a *App) pecasDaVisao(ia *situacaoIA) []pecaDaVisao {
	ligado := a.cfg.Ligado && a.cfg.Salvo
	atendente := pecaDaVisao{Chave: "atendente", Certo: ligado, Titulo: "Atendente ligado", Detalhe: "Ele responde as conversas privadas do WhatsApp do negócio."}
	if !a.cfg.Salvo {
		atendente.Titulo, atendente.Detalhe = "Configuração inicial por fazer", "Conte sobre o negócio para o assistente poder responder."
	} else if !ligado {
		atendente.Titulo, atendente.Detalhe = "Atendente pausado", "Ninguém recebe resposta automática. Ligue na chave do topo."
	}

	whats := pecaDaVisao{Chave: "whatsapp", Certo: a.status == "conectado", Titulo: "WhatsApp conectado"}
	if whats.Certo {
		whats.Detalhe = ouPadrao(a.nomeConta, "Número do negócio")
		if n := telefoneBonito(a.numConta); n != "" {
			whats.Detalhe += ", " + n
		}
	} else {
		whats.Titulo, whats.Detalhe = "WhatsApp desconectado", "Leia o código QR em Ajustes para conectar o número do negócio."
	}

	cerebro := pecaDaVisao{Chave: "ia", Certo: true, Titulo: "IA conectada", Detalhe: "O Claude está pensando as respostas."}
	switch {
	case ia == nil:
		cerebro.Titulo, cerebro.Detalhe = "Conferindo a IA", "Um instante."
	case ia.Provedor == provedorOpenAI && ia.Conectado:
		cerebro.Detalhe = "O ChatGPT está pensando as respostas."
	case !ia.Instalado:
		cerebro.Certo, cerebro.Titulo, cerebro.Detalhe = false, "Falta o Claude Code", "Sem ele o assistente não responde. Veja em Ajustes, Cérebro."
	case !ia.Conectado:
		cerebro.Certo, cerebro.Titulo, cerebro.Detalhe = false, "IA desconectada", "O assistente não responde. Clique em Conectar a IA."
	}

	agenda := pecaDaVisao{Chave: "agenda", Certo: a.cfg.Agenda.Ativa, Titulo: "Agenda ligada", Detalhe: "O assistente marca, remarca e desmarca os horários sozinho."}
	if !a.cfg.Agenda.Ativa {
		agenda.Titulo, agenda.Detalhe = "Agenda desligada", "O assistente só informa os horários. Ligue na aba Agenda se o negócio atende com hora marcada."
	}
	return []pecaDaVisao{atendente, whats, cerebro, agenda}
}

// rotaVisao entrega a pagina Inicio.
func (a *App) rotaVisao(w http.ResponseWriter, r *http.Request) {
	agora := time.Now()
	var ia *situacaoIA
	if e, pronto := a.conexao.guardado(); pronto {
		s := a.situacaoDaIA(e)
		ia = &s
	}

	materiais := a.base.total() // a base tem a trava dela: fica fora da trava do App

	a.mu.Lock()
	leads := a.leadsOrdenados() // do mais recente para o mais antigo
	hoje := montarRelatorio(leads, a.agenda, 1, agora)
	semana := montarRelatorio(leads, a.agenda, 7, agora)

	fila := []esperaDaVisao{}
	recentes := []conversaDaVisao{}
	for _, l := range leads {
		if l.Status == statusHumano {
			desde := l.UltimaMsg
			for i := len(l.Historico) - 1; i >= 0; i-- {
				if m := l.Historico[i]; m.De == "sistema" && m.Evento == "transbordo" {
					desde = m.Hora
					break
				}
			}
			fila = append(fila, esperaDaVisao{ID: l.ID, Exibir: l.exibir(), Motivo: l.Motivo, Desde: desde, Classe: ouPadrao(l.Classificacao, "indefinido")})
		}
		if len(recentes) < 6 {
			v := a.visaoLead(l, agora)
			recentes = append(recentes, conversaDaVisao{
				ID: l.ID, Exibir: v.Exibir, Previa: v.Previa, PreviaDe: v.PreviaDe, UltimaMsg: l.UltimaMsg,
				Classe: v.Classificacao, Situacao: situacaoDe(l),
			})
		}
	}
	// Quem espera ha mais tempo vem primeiro.
	sort.SliceStable(fila, func(i, j int) bool { return fila[i].Desde.Before(fila[j].Desde) })
	totalNaFila := len(fila)
	if len(fila) > 6 {
		fila = fila[:6]
	}

	proximos := []agendamentoVisao{}
	marcadosHoje := 0
	fimDoDia := time.Date(agora.Year(), agora.Month(), agora.Day(), 23, 59, 59, 0, agora.Location())
	for _, ag := range a.agenda {
		if ag.Status == "cancelado" || !ag.fim().After(agora) {
			continue
		}
		proximos = append(proximos, visaoAgendamento(ag))
		if !ag.Inicio.After(fimDoDia) {
			marcadosHoje++
		}
	}
	sort.Slice(proximos, func(i, j int) bool { return proximos[i].Inicio.Before(proximos[j].Inicio) })
	totalDeProximos := len(proximos)
	if len(proximos) > 5 {
		proximos = proximos[:5]
	}

	resp := map[string]any{
		"agora":             agora,
		"no_ar_desde":       a.iniciado,
		"negocio":           a.cfg.Negocio,
		"pecas":             a.pecasDaVisao(ia),
		"hoje":              hoje["periodo"],
		"semana":            semana["periodo"],
		"situacao":          hoje["agora"],
		"por_dia":           semana["por_dia"],
		"fila":              fila,
		"fila_total":        totalNaFila,
		"proximos":          proximos,
		"proximos_total":    totalDeProximos,
		"marcados_hoje":     marcadosHoje,
		"recentes":          recentes,
		"ultima_mensagem":   ultimaDe(leads, "cliente"),
		"ultima_resposta":   ultimaDe(leads, "atendente"),
		"materiais":         materiais,
		"etiquetas_em_uso":  len(a.etiquetasEmUso()),
		"contatos_no_total": len(a.leads),
	}
	a.mu.Unlock()
	responderJSON(w, http.StatusOK, resp)
}
