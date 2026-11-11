package main

import (
	"os"
	"strings"
	"testing"
	"time"
)

// pastaDeTeste leva o teste para uma pasta vazia, para nada ser gravado nos dados de verdade.
func pastaDeTeste(t *testing.T) {
	t.Helper()
	t.Chdir(t.TempDir())
	for _, p := range []string{pastaDados, pastaTextos, pastaConhecimento} {
		if err := os.MkdirAll(p, 0o700); err != nil {
			t.Fatal(err)
		}
	}
}

func agendaDeTeste() AgendaConfig {
	c := AgendaConfig{Ativa: true, Servicos: []Servico{{Nome: "Corte", DuracaoMin: 30}, {Nome: "Corte e barba", DuracaoMin: 60}}}
	c.normalizar() // seg a sex 09 as 18, sabado 09 as 13, 30 em 30 min, 1 vaga, 60 min de antecedencia
	return c
}

func hora(dia int, hhmm string) time.Time {
	m, _ := minutos(hhmm)
	return time.Date(2026, 11, dia, m/60, m%60, 0, 0, time.Local) // 11/11/2026 e quarta-feira
}

func tem(lista []string, item string) bool {
	for _, v := range lista {
		if v == item {
			return true
		}
	}
	return false
}

func TestHorariosLivres(t *testing.T) {
	c := agendaDeTeste()
	agora := hora(11, "08:00")
	ags := []*Agendamento{{Status: "marcado", Inicio: hora(11, "10:00"), DuracaoMin: 30}}

	livres := c.horariosLivres(ags, agora, 30, agora)
	if !tem(livres, "09:00") || !tem(livres, "17:30") {
		t.Errorf("faltou o primeiro ou o último horário do dia: %v", livres)
	}
	if tem(livres, "10:00") {
		t.Errorf("10:00 já está ocupado e apareceu como livre")
	}
	if tem(livres, "18:00") {
		t.Errorf("18:00 passa do fim do expediente")
	}

	// Um servico de 60 minutos nao cabe as 09:30 (bate no das 10:00) nem as 17:30.
	longos := c.horariosLivres(ags, agora, 60, agora)
	for _, ruim := range []string{"09:30", "10:00", "17:30"} {
		if tem(longos, ruim) {
			t.Errorf("serviço de 60 min não cabe às %s", ruim)
		}
	}
	if !tem(longos, "10:30") {
		t.Errorf("serviço de 60 min cabe às 10:30: %v", longos)
	}

	// Um cancelado nao ocupa o horario.
	ags[0].Status = "cancelado"
	if !tem(c.horariosLivres(ags, agora, 30, agora), "10:00") {
		t.Errorf("horário cancelado deveria voltar a ficar livre")
	}

	// Domingo fechado.
	if livres := c.horariosLivres(nil, hora(15, "08:00"), 30, agora); len(livres) != 0 {
		t.Errorf("domingo está fechado, mas vieram horários: %v", livres)
	}
}

func TestPausaEAntecedencia(t *testing.T) {
	c := agendaDeTeste()
	c.Dias[3].PausaInicio, c.Dias[3].PausaFim = "12:00", "13:00"
	c.normalizar()
	agora := hora(11, "09:10")
	livres := c.horariosLivres(nil, agora, 30, agora)
	for _, ruim := range []string{"09:00", "09:30", "10:00", "12:00", "12:30"} {
		if tem(livres, ruim) {
			t.Errorf("%s não podia estar livre (passou, está em cima da hora ou é pausa): %v", ruim, livres)
		}
	}
	if !tem(livres, "10:30") || !tem(livres, "11:30") || !tem(livres, "13:00") {
		t.Errorf("faltaram horários válidos: %v", livres)
	}
	if tem(c.horariosLivres(nil, agora, 60, agora), "11:30") {
		t.Errorf("serviço de 60 min às 11:30 invade a pausa")
	}
}

func TestVagasEPainel(t *testing.T) {
	c := agendaDeTeste()
	c.Vagas = 2
	agora := hora(11, "08:00")
	ags := []*Agendamento{{Status: "marcado", Inicio: hora(11, "10:00"), DuracaoMin: 30}}
	if err := c.conferir(ags, hora(11, "10:00"), 30, agora, false); err != nil {
		t.Errorf("com 2 vagas ainda cabe mais um às 10:00: %v", err)
	}
	ags = append(ags, &Agendamento{Status: "marcado", Inicio: hora(11, "10:00"), DuracaoMin: 30})
	if err := c.conferir(ags, hora(11, "10:00"), 30, agora, false); err != errHorarioCheio {
		t.Errorf("com as 2 vagas ocupadas devia dar horário cheio, deu: %v", err)
	}
	// Pelo painel a pessoa marca fora do expediente, mas nao em horario cheio nem no passado.
	if err := c.conferir(ags, hora(11, "20:00"), 30, agora, true); err != nil {
		t.Errorf("o painel pode marcar fora do expediente: %v", err)
	}
	if err := c.conferir(ags, hora(11, "20:00"), 30, agora, false); err == nil {
		t.Errorf("o assistente não pode marcar fora do expediente")
	}
	if err := c.conferir(ags, hora(11, "10:00"), 30, agora, true); err != errHorarioCheio {
		t.Errorf("nem o painel marca em horário cheio: %v", err)
	}
	if err := c.conferir(nil, hora(10, "10:00"), 30, agora, true); err == nil {
		t.Errorf("ninguém marca no passado")
	}
	if err := c.conferir(nil, hora(25, "10:00"), 30, agora, false); err == nil {
		t.Errorf("o assistente não marca além dos dias configurados")
	}
}

func TestInterpretarDataHora(t *testing.T) {
	agora := hora(11, "08:00")
	casos := []struct{ data, hora, quer string }{
		{"2026-11-12", "14:00", "2026-11-12 14:00"},
		{"12/11/2026", "14h30", "2026-11-12 14:30"},
		{"12/11", "14h", "2026-11-12 14:00"},
		{"12/11", "9", "2026-11-12 09:00"},
		{"05/01", "10:00", "2027-01-05 10:00"}, // ja passou neste ano: vale o proximo
	}
	for _, c := range casos {
		tm, err := interpretarDataHora(c.data, c.hora, agora)
		if err != nil || tm.Format("2006-01-02 15:04") != c.quer {
			t.Errorf("interpretarDataHora(%q, %q) = %v, %v; quero %s", c.data, c.hora, tm.Format("2006-01-02 15:04"), err, c.quer)
		}
	}
	for _, ruim := range [][2]string{{"amanhã", "10:00"}, {"2026-11-12", "tarde"}, {"2026-11-12", "25:00"}} {
		if _, err := interpretarDataHora(ruim[0], ruim[1], agora); err == nil {
			t.Errorf("interpretarDataHora(%q, %q) devia falhar", ruim[0], ruim[1])
		}
	}
}

func TestServicoDe(t *testing.T) {
	c := agendaDeTeste()
	casos := []struct {
		texto, nome string
		dur         int
	}{
		{"corte", "Corte", 30},
		{"CORTE E BARBA", "Corte e barba", 60},
		{"barba", "Corte e barba", 60},
		{"massagem", "massagem", 30}, // nao existe: mantem o nome e usa a duracao padrao
	}
	for _, caso := range casos {
		nome, dur := c.servicoDe(caso.texto)
		if nome != caso.nome || dur != caso.dur {
			t.Errorf("servicoDe(%q) = %q, %d; quero %q, %d", caso.texto, nome, dur, caso.nome, caso.dur)
		}
	}
}

func TestAplicarAgendamento(t *testing.T) {
	pastaDeTeste(t)
	a := novoApp()
	a.cfg.Agenda = agendaDeTeste()
	a.cfg.normalizar()
	agora := hora(11, "08:00")
	l := &Lead{ID: "5527999990000@s.whatsapp.net", Telefone: "5527999990000", NomeWhats: "Carlão"}
	a.leads[l.ID] = l

	// Marca.
	r := a.aplicarAgendamento(l, AgendamentoIA{Acao: "marcar", Servico: "corte", Data: "2026-11-12", Hora: "14:00"}, "Carlos", agora)
	if r.Marcado == nil || r.Resposta != "" {
		t.Fatalf("devia marcar sem trocar a resposta: %+v", r)
	}
	if r.Marcado.Nome != "Carlos" || r.Marcado.Servico != "Corte" || r.Marcado.Origem != "assistente" {
		t.Errorf("agendamento com dados errados: %+v", r.Marcado)
	}
	// A IA repetiu o mesmo pedido na mensagem seguinte: nao duplica.
	r = a.aplicarAgendamento(l, AgendamentoIA{Acao: "marcar", Servico: "corte", Data: "2026-11-12", Hora: "14:00"}, "Carlos", agora)
	if r.Marcado != nil || r.Resposta != "" || len(a.agenda) != 1 {
		t.Errorf("pedido repetido não pode duplicar: %+v, total %d", r, len(a.agenda))
	}
	// Outra pessoa quer o mesmo horario: a resposta e trocada por opcoes livres.
	outro := &Lead{ID: "5527988880000@s.whatsapp.net", Telefone: "5527988880000"}
	r = a.aplicarAgendamento(outro, AgendamentoIA{Acao: "marcar", Servico: "corte", Data: "2026-11-12", Hora: "14:00"}, "Bia", agora)
	if r.Marcado != nil || !strings.Contains(r.Resposta, "não está disponível") || !strings.Contains(r.Resposta, "qui 12/11") {
		t.Errorf("horário ocupado devia oferecer outros do mesmo dia: %+v", r)
	}
	// Data que a IA escreveu torta: pede de novo, nao marca.
	r = a.aplicarAgendamento(outro, AgendamentoIA{Acao: "marcar", Servico: "corte", Data: "quinta", Hora: "14:00"}, "Bia", agora)
	if r.Marcado != nil || r.Resposta == "" {
		t.Errorf("data ilegível devia pedir o dia de novo: %+v", r)
	}
	// Cancela.
	r = a.aplicarAgendamento(l, AgendamentoIA{Acao: "cancelar", Data: "2026-11-12", Hora: "14:00"}, "", agora)
	if r.Cancelado == nil || a.agenda[0].Status != "cancelado" {
		t.Errorf("devia cancelar o horário do cliente: %+v", r)
	}
	// Agenda desligada: nada acontece.
	a.cfg.Agenda.Ativa = false
	r = a.aplicarAgendamento(l, AgendamentoIA{Acao: "marcar", Servico: "corte", Data: "2026-11-13", Hora: "10:00"}, "Carlos", agora)
	if r.Marcado != nil || r.Resposta != "" {
		t.Errorf("com a agenda desligada nada devia acontecer: %+v", r)
	}
}
