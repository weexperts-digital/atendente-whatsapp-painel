package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestHorarioPorExtenso(t *testing.T) {
	c := AgendaConfig{}.padrao()
	c.normalizar()
	if got := c.horarioPorExtenso(); got != "Segunda a sexta, das 9h às 18h. Sábado, das 9h às 13h. Domingo, fechado." {
		t.Errorf("semana padrão: %q", got)
	}

	// O caso do teste do dono: almoço diferente no sábado, que também abre mais cedo.
	for d := 1; d <= 5; d++ {
		c.Dias[d] = Expediente{Dia: d, Aberto: true, Inicio: "09:00", Fim: "19:00", PausaInicio: "12:00", PausaFim: "13:30"}
	}
	c.Dias[6] = Expediente{Dia: 6, Aberto: true, Inicio: "08:00", Fim: "20:00", PausaInicio: "12:00", PausaFim: "14:00"}
	c.normalizar()
	quer := "Segunda a sexta, das 9h às 19h, com pausa das 12h às 13h30. Sábado, das 8h às 20h, com pausa das 12h às 14h. Domingo, fechado."
	if got := c.horarioPorExtenso(); got != quer {
		t.Errorf("com almoço:\n tem %q\nquer %q", got, quer)
	}

	// Dois dias iguais seguidos usam "e"; semana toda fechada não tem texto.
	c.Dias[0] = c.Dias[6]
	if got := c.horarioPorExtenso(); !strings.Contains(got, "Sábado e domingo, das 8h às 20h") {
		t.Errorf("sábado e domingo iguais: %q", got)
	}
	for d := range c.Dias {
		c.Dias[d].Aberto = false
	}
	if got := c.horarioPorExtenso(); got != "" {
		t.Errorf("tudo fechado devia ficar vazio: %q", got)
	}
}

func TestAgendaDaIA(t *testing.T) {
	// O que a IA devolve para "seg a sex 9h às 19h com almoço 12h às 13h30, sábado 8h às 20h com almoço 12h às 14h".
	bruto := map[string]json.RawMessage{
		"usa_agenda": json.RawMessage(`true`),
		"dias": json.RawMessage(`[
			{"dia":0,"aberto":false,"inicio":"","fim":"","pausa_inicio":"","pausa_fim":""},
			{"dia":1,"aberto":true,"inicio":"09:00","fim":"19:00","pausa_inicio":"12:00","pausa_fim":"13:30"},
			{"dia":2,"aberto":true,"inicio":"09:00","fim":"19:00","pausa_inicio":"12:00","pausa_fim":"13:30"},
			{"dia":3,"aberto":true,"inicio":"09:00","fim":"19:00","pausa_inicio":"12:00","pausa_fim":"13:30"},
			{"dia":4,"aberto":true,"inicio":"09:00","fim":"19:00","pausa_inicio":"12:00","pausa_fim":"13:30"},
			{"dia":5,"aberto":true,"inicio":"09:00","fim":"19:00","pausa_inicio":"12:00","pausa_fim":"13:30"},
			{"dia":6,"aberto":true,"inicio":"08:00","fim":"20:00","pausa_inicio":"12:00","pausa_fim":"14:00"}]`),
		"servicos": json.RawMessage(`[{"nome":"Corte","duracao_min":30,"preco":"R$ 70"},{"nome":"Corte com barba","duracao_min":0,"preco":"R$ 100"},{"nome":"  ","duracao_min":30,"preco":""}]`),
	}
	campos := map[string]string{"horario": "texto antigo"}
	ag := agendaDaIA(bruto, campos)
	if ag == nil || !ag.Usa || len(ag.Dias) != 7 || len(ag.Servicos) != 2 {
		t.Fatalf("agenda entendida: %+v", ag)
	}
	if d := ag.Dias[6]; !d.Aberto || d.Inicio != "08:00" || d.Fim != "20:00" || d.PausaInicio != "12:00" || d.PausaFim != "14:00" {
		t.Errorf("sábado: %+v", d)
	}
	if d := ag.Dias[3]; d.PausaInicio != "12:00" || d.PausaFim != "13:30" {
		t.Errorf("quarta: %+v", d)
	}
	if ag.Dias[0].Aberto {
		t.Errorf("domingo devia ficar fechado")
	}
	if ag.Servicos[1].DuracaoMin <= 0 || ag.Servicos[0].Preco != "R$ 70" {
		t.Errorf("serviços: %+v", ag.Servicos)
	}
	if !strings.HasPrefix(campos["horario"], "Segunda a sexta, das 9h às 19h, com pausa das 12h às 13h30. Sábado, das 8h às 20h") {
		t.Errorf("o texto do horário devia sair dos dias: %q", campos["horario"])
	}

	// Sem horário e sem serviço: nada de agenda. Só serviços: aproveita os serviços.
	if agendaDaIA(map[string]json.RawMessage{"dias": json.RawMessage(`[]`), "servicos": json.RawMessage(`[]`)}, map[string]string{}) != nil {
		t.Errorf("descrição sem horário não gera agenda")
	}
	so := agendaDaIA(map[string]json.RawMessage{"dias": json.RawMessage(`[]`), "servicos": json.RawMessage(`[{"nome":"Consulta","duracao_min":60,"preco":""}]`)}, map[string]string{})
	if so == nil || so.Dias != nil || len(so.Servicos) != 1 || !so.Usa {
		t.Errorf("só serviços: %+v", so)
	}
	// Semana incompleta ou repetida não é aceita como horário.
	if x := agendaDaIA(map[string]json.RawMessage{"dias": json.RawMessage(`[{"dia":1,"aberto":true,"inicio":"09:00","fim":"18:00","pausa_inicio":"","pausa_fim":""},{"dia":1,"aberto":true,"inicio":"10:00","fim":"12:00","pausa_inicio":"","pausa_fim":""}]`)}, map[string]string{}); x != nil {
		t.Errorf("semana incompleta devia ser ignorada: %+v", x)
	}
}

func TestSalvarAgendaAtualizaOHorarioDoAssistente(t *testing.T) {
	a := novoApp()
	a.cfg.Negocio, a.cfg.Salvo, a.cfg.Horario = "Barbearia Exemplo", true, "texto antigo, digitado à mão"

	postarConfig := func(corpo string) {
		req := httptest.NewRequest(http.MethodPost, "/api/config", strings.NewReader(corpo))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		a.rotaConfig(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("salvar: %d %s", w.Code, w.Body.String())
		}
	}
	// Salvar outra coisa não mexe no horário.
	postarConfig(`{"cidade":"Vitória"}`)
	if a.cfg.Horario != "texto antigo, digitado à mão" {
		t.Errorf("salvar sem agenda não pode trocar o horário: %q", a.cfg.Horario)
	}
	// Salvar as regras da agenda escreve o horário a partir dos dias e liga a agenda.
	postarConfig(`{"agenda":{"ativa":true,"servicos":[{"nome":"Corte","duracao_min":30,"preco":"R$ 70"}],"dias":[
		{"dia":0,"aberto":false,"inicio":"09:00","fim":"18:00"},
		{"dia":1,"aberto":true,"inicio":"09:00","fim":"19:00","pausa_inicio":"12:00","pausa_fim":"13:30"},
		{"dia":2,"aberto":true,"inicio":"09:00","fim":"19:00","pausa_inicio":"12:00","pausa_fim":"13:30"},
		{"dia":3,"aberto":true,"inicio":"09:00","fim":"19:00","pausa_inicio":"12:00","pausa_fim":"13:30"},
		{"dia":4,"aberto":true,"inicio":"09:00","fim":"19:00","pausa_inicio":"12:00","pausa_fim":"13:30"},
		{"dia":5,"aberto":true,"inicio":"09:00","fim":"19:00","pausa_inicio":"12:00","pausa_fim":"13:30"},
		{"dia":6,"aberto":true,"inicio":"08:00","fim":"20:00","pausa_inicio":"12:00","pausa_fim":"14:00"}]}}`)
	if !a.cfg.Agenda.Ativa || len(a.cfg.Agenda.Servicos) != 1 {
		t.Fatalf("a agenda devia ficar ligada com o serviço: %+v", a.cfg.Agenda)
	}
	if !strings.Contains(a.cfg.Horario, "Sábado, das 8h às 20h, com pausa das 12h às 14h") {
		t.Errorf("o horário informado pelo assistente devia seguir a agenda: %q", a.cfg.Horario)
	}

	// E a agenda respeita o almoço e o sábado que abre mais cedo (14/11/2026 é sábado).
	agora := time.Date(2026, 11, 12, 10, 0, 0, 0, time.Local)
	sabado := time.Date(2026, 11, 14, 0, 0, 0, 0, time.Local)
	livres := strings.Join(a.cfg.Agenda.horariosLivres(nil, sabado, 30, agora), ",")
	if !strings.HasPrefix(livres, "08:00,08:30") || strings.Contains(livres, "12:00") || strings.Contains(livres, "13:30") || !strings.Contains(livres, "14:00") || !strings.HasSuffix(livres, "19:30") {
		t.Errorf("sábado: horários livres %s", livres)
	}
	sexta := time.Date(2026, 11, 13, 0, 0, 0, 0, time.Local)
	livres = strings.Join(a.cfg.Agenda.horariosLivres(nil, sexta, 30, agora), ",")
	if !strings.HasPrefix(livres, "09:00") || strings.Contains(livres, "12:30") || strings.Contains(livres, "13:00") || !strings.Contains(livres, "13:30") || !strings.HasSuffix(livres, "18:30") {
		t.Errorf("sexta: horários livres %s", livres)
	}
}

func TestAssistenteRemarca(t *testing.T) {
	agora := time.Date(2026, 11, 11, 10, 0, 0, 0, time.Local) // quarta
	a := novoApp()
	a.cfg.Agenda = agendaDeTeste()
	l := &Lead{ID: "5527999990001@s.whatsapp.net", Telefone: "5527999990001", Nome: "Carlos"}
	outro := &Lead{ID: "5527999990002@s.whatsapp.net", Telefone: "5527999990002", Nome: "Bia"}
	a.leads[l.ID], a.leads[outro.ID] = l, outro

	if r := a.aplicarAgendamento(l, AgendamentoIA{Acao: "marcar", Servico: "corte", Data: "2026-11-12", Hora: "14:00"}, "Carlos", agora); r.Marcado == nil {
		t.Fatalf("devia marcar: %+v", r)
	}
	if r := a.aplicarAgendamento(outro, AgendamentoIA{Acao: "marcar", Servico: "corte", Data: "2026-11-12", Hora: "15:00"}, "Bia", agora); r.Marcado == nil {
		t.Fatalf("devia marcar o da Bia: %+v", r)
	}

	// Trocar para um horário livre: o mesmo agendamento muda de lugar, sem criar outro.
	r := a.aplicarAgendamento(l, AgendamentoIA{Acao: "remarcar", Servico: "corte", Data: "2026-11-13", Hora: "10:00"}, "Carlos", agora)
	if r.Remarcado == nil || r.Remarcado.Inicio.Format("2006-01-02 15:04") != "2026-11-13 10:00" || r.Antes.Format("15:04") != "14:00" {
		t.Fatalf("devia remarcar: %+v", r)
	}
	if n := len(a.futurosDoLead(l.ID, agora)); n != 1 {
		t.Errorf("remarcar não pode criar um segundo agendamento, ficaram %d", n)
	}
	if !strings.Contains(avisoRemarcacao(r.Remarcado, r.Antes), "REMARCADO") {
		t.Errorf("aviso de remarcação sem o título")
	}

	// Horário ocupado por outra pessoa: não troca, oferece opções e mantém o atual.
	r = a.aplicarAgendamento(l, AgendamentoIA{Acao: "remarcar", Data: "2026-11-12", Hora: "15:00"}, "Carlos", agora)
	if r.Remarcado != nil || !strings.Contains(r.Resposta, "continua marcado") {
		t.Errorf("horário ocupado: %+v", r)
	}
	if f := a.futurosDoLead(l.ID, agora); len(f) != 1 || f[0].Inicio.Format("2006-01-02 15:04") != "2026-11-13 10:00" {
		t.Errorf("o horário atual tinha de continuar")
	}

	// Voltar para o próprio horário antigo dele funciona (a vaga dele mesmo não conta como ocupada).
	if r = a.aplicarAgendamento(l, AgendamentoIA{Acao: "remarcar", Data: "2026-11-12", Hora: "14:00"}, "Carlos", agora); r.Remarcado == nil {
		t.Errorf("devia voltar para as 14h: %+v", r)
	}
	// Data que não dá para entender: pede de novo, sem mexer em nada.
	if r = a.aplicarAgendamento(l, AgendamentoIA{Acao: "remarcar", Data: "", Hora: ""}, "Carlos", agora); r.Remarcado != nil || r.Resposta == "" {
		t.Errorf("sem data: %+v", r)
	}
	// Remarcar sem ter nada marcado vira um marcar comum.
	novo := &Lead{ID: "5527999990003@s.whatsapp.net", Telefone: "5527999990003", Nome: "Davi"}
	a.leads[novo.ID] = novo
	if r = a.aplicarAgendamento(novo, AgendamentoIA{Acao: "remarcar", Servico: "corte", Data: "2026-11-13", Hora: "11:00"}, "Davi", agora); r.Marcado == nil {
		t.Errorf("remarcar sem agendamento devia marcar: %+v", r)
	}

	// O formato da resposta da IA aceita a ação nova e as instruções ensinam a usar.
	if !strings.Contains(esquemaSaida, `"remarcar"`) {
		t.Errorf("o esquema precisa aceitar a ação remarcar")
	}
	bloco := a.blocoAgenda(a.cfg, l.ID, agora)
	if !strings.Contains(bloco, `acao "remarcar"`) || !strings.Contains(bloco, "nunca diga que um atendente vai confirmar o horário") {
		t.Errorf("as instruções da agenda precisam ensinar a remarcar e a não empurrar para a equipe")
	}
}
