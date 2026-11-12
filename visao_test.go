package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestPaginaInicio(t *testing.T) {
	a := novoApp()
	agora := time.Now()
	a.cfg.Negocio, a.cfg.Salvo, a.cfg.Ligado = "Clínica Exemplo", true, true
	a.cfg.Agenda = agendaDeTeste()
	a.status, a.nomeConta, a.numConta = "conectado", "Clínica Exemplo", "5527999990000"

	// Uma conversa com o assistente, uma na fila há mais tempo e outra na fila há pouco.
	a.leads["a"] = &Lead{ID: "a", Telefone: "5527999990001", Nome: "Ana", Classificacao: "morno", Status: statusQualificando, CriadoEm: agora.Add(-2 * time.Hour), UltimaMsg: agora.Add(-5 * time.Minute),
		Historico: []Msg{{De: "cliente", Texto: "Oi", Hora: agora.Add(-6 * time.Minute)}, {De: "atendente", Texto: "Olá! Posso ajudar?", Hora: agora.Add(-5 * time.Minute)}}}
	a.leads["b"] = &Lead{ID: "b", Telefone: "5527999990002", Nome: "Bia", Classificacao: "quente", Status: statusHumano, Motivo: "pediu uma pessoa", CriadoEm: agora.Add(-3 * time.Hour), UltimaMsg: agora.Add(-40 * time.Minute),
		Historico: []Msg{{De: "cliente", Texto: "Quero falar com alguém", Hora: agora.Add(-41 * time.Minute)}, {De: "sistema", Tipo: "evento", Evento: "transbordo", Texto: "Transferido", Hora: agora.Add(-40 * time.Minute)}}}
	a.leads["c"] = &Lead{ID: "c", Telefone: "5527999990003", Nome: "Caio", Classificacao: "quente", Status: statusHumano, Motivo: "reclamação", CriadoEm: agora.Add(-26 * time.Hour), UltimaMsg: agora.Add(-10 * time.Minute),
		Historico: []Msg{{De: "cliente", Texto: "Não gostei", Hora: agora.Add(-11 * time.Minute)}, {De: "sistema", Tipo: "evento", Evento: "transbordo", Texto: "Transferido", Hora: agora.Add(-10 * time.Minute)}}}
	a.agenda = []*Agendamento{
		{ID: "1", LeadID: "a", Nome: "Ana", Servico: "Corte", Inicio: agora.Add(48 * time.Hour), DuracaoMin: 30, Status: "marcado", CriadoEm: agora},
		{ID: "2", LeadID: "b", Nome: "Bia", Servico: "Corte", Inicio: agora.Add(24 * time.Hour), DuracaoMin: 30, Status: "marcado", CriadoEm: agora},
		{ID: "3", LeadID: "c", Nome: "Caio", Servico: "Corte", Inicio: agora.Add(72 * time.Hour), DuracaoMin: 30, Status: "cancelado", CriadoEm: agora},
		{ID: "4", LeadID: "a", Nome: "Ana", Servico: "Corte", Inicio: agora.Add(-24 * time.Hour), DuracaoMin: 30, Status: "marcado", CriadoEm: agora.Add(-48 * time.Hour)},
	}

	w := httptest.NewRecorder()
	a.rotaVisao(w, httptest.NewRequest(http.MethodGet, "/api/visao", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("página Início: %d %s", w.Code, w.Body.String())
	}
	var r struct {
		Negocio string        `json:"negocio"`
		Pecas   []pecaDaVisao `json:"pecas"`
		Hoje    map[string]int
		Semana  map[string]int
		Sit     map[string]int `json:"situacao"`
		PorDia  []diaRelatorio `json:"por_dia"`
		Fila    []esperaDaVisao
		FilaN   int                `json:"fila_total"`
		Prox    []agendamentoVisao `json:"proximos"`
		ProxN   int                `json:"proximos_total"`
		Rec     []conversaDaVisao  `json:"recentes"`
		UltResp time.Time          `json:"ultima_resposta"`
		UltMsg  time.Time          `json:"ultima_mensagem"`
		Total   int                `json:"contatos_no_total"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &r); err != nil {
		t.Fatal(err)
	}
	if r.Negocio != "Clínica Exemplo" || r.Total != 3 {
		t.Errorf("cabeçalho: %q, %d contatos", r.Negocio, r.Total)
	}
	// As quatro peças, na ordem, com o estado certo. A IA ainda não foi conferida: não acusa erro.
	if len(r.Pecas) != 4 || r.Pecas[0].Chave != "atendente" || !r.Pecas[0].Certo || !r.Pecas[1].Certo || r.Pecas[1].Chave != "whatsapp" || !r.Pecas[3].Certo || r.Pecas[2].Titulo != "Conferindo a IA" {
		t.Errorf("peças: %+v", r.Pecas)
	}
	if r.Hoje["mensagens_clientes"] != 3 || r.Hoje["respostas_assistente"] != 1 || r.Hoje["transbordos"] != 2 || r.Semana["mensagens_clientes"] != 3 {
		t.Errorf("números de hoje %v e da semana %v", r.Hoje, r.Semana)
	}
	if r.Sit["aguardando"] != 2 || r.Sit["quentes"] != 2 || r.Sit["total"] != 3 || len(r.PorDia) != 7 {
		t.Errorf("situação %v, %d dias", r.Sit, len(r.PorDia))
	}
	// Na fila, quem espera há mais tempo vem primeiro.
	if r.FilaN != 2 || len(r.Fila) != 2 || r.Fila[0].Exibir != "Bia" || r.Fila[0].Motivo != "pediu uma pessoa" || r.Fila[1].Exibir != "Caio" {
		t.Errorf("fila: %+v", r.Fila)
	}
	// Próximos horários: só os marcados e que ainda vão acontecer, do mais perto para o mais longe.
	if r.ProxN != 2 || len(r.Prox) != 2 || r.Prox[0].Nome != "Bia" || r.Prox[1].Nome != "Ana" {
		t.Errorf("próximos horários: %+v", r.Prox)
	}
	if len(r.Rec) != 3 || r.Rec[0].Exibir != "Ana" || r.Rec[0].Situacao != "assistente" || r.Rec[1].Situacao != "fila" {
		t.Errorf("últimas conversas: %+v", r.Rec)
	}
	if r.UltResp.IsZero() || r.UltMsg.IsZero() || r.UltResp.Before(agora.Add(-6*time.Minute)) {
		t.Errorf("última resposta %v, última mensagem %v", r.UltResp, r.UltMsg)
	}

	// Atendente pausado e WhatsApp fora do ar aparecem como problema, com o que fazer.
	a.cfg.Ligado, a.status, a.cfg.Agenda.Ativa = false, "desconectado", false
	w = httptest.NewRecorder()
	a.rotaVisao(w, httptest.NewRequest(http.MethodGet, "/api/visao", nil))
	_ = json.Unmarshal(w.Body.Bytes(), &r)
	if r.Pecas[0].Certo || r.Pecas[0].Titulo != "Atendente pausado" || r.Pecas[1].Certo || r.Pecas[3].Certo || r.Pecas[1].Detalhe == "" {
		t.Errorf("peças com problema: %+v", r.Pecas)
	}
	if nivelDaRota("GET", "/api/visao") != nivelLogado {
		t.Errorf("a página Início é para quem está logado")
	}
}
