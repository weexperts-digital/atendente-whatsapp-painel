package main

import (
	"strings"
	"testing"
	"time"
)

func TestArquivosDeMidia(t *testing.T) {
	casos := []struct{ mime, nome, quer string }{
		{"image/jpeg", "", ".jpg"},
		{"audio/ogg; codecs=opus", "", ".ogg"},
		{"application/pdf", "tabela.PDF", ".pdf"},
		{"application/vnd.ms-excel", "planilha.xls", ".xls"},
		{"text/html", "pagina.html", ".bin"}, // pode carregar script: vira arquivo generico
		{"image/svg+xml", "logo.svg", ".bin"},
		{"", "sem extensao", ".bin"},
		{"", "../../etc/passwd", ".bin"},
	}
	for _, c := range casos {
		if tem := extensaoDe(c.mime, c.nome); tem != c.quer {
			t.Errorf("extensaoDe(%q, %q) = %q, quero %q", c.mime, c.nome, tem, c.quer)
		}
	}
	for _, bom := range []string{"f0123456789.jpg", "fabcdef0123.pdf"} {
		if _, ok := caminhoMidia(bom); !ok {
			t.Errorf("%q devia ser aceito", bom)
		}
	}
	for _, ruim := range []string{"", "../config.json", "f0123456789.jpg/../../x", "x0123456789.jpg", "f0123456789", "f0123456789.JPG", "f0123456789.tar.gz"} {
		if _, ok := caminhoMidia(ruim); ok {
			t.Errorf("%q devia ser recusado", ruim)
		}
	}
	if _, err := baixarFoto(t.Context(), "https://exemplo.com/foto.jpg"); err == nil {
		t.Errorf("foto só pode vir dos servidores do WhatsApp")
	}
	if _, err := baixarFoto(t.Context(), "http://pps.whatsapp.net/foto.jpg"); err == nil {
		t.Errorf("foto só pode vir por https")
	}
}

func TestEtiquetasENumeros(t *testing.T) {
	tem := limparEtiquetas([]string{" VIP ", "vip", "", "Orçamento  grande", strings.Repeat("x", 50)})
	if len(tem) != 3 || tem[0] != "VIP" || tem[1] != "Orçamento grande" || len([]rune(tem[2])) != 30 {
		t.Errorf("etiquetas mal limpas: %q", tem)
	}

	// Config antiga, com um numero so, vira lista. Repetidos (com e sem o nono digito) saem.
	c := Config{NumeroHumano: "55 27 99999-1111"}
	c.normalizar()
	if len(c.NumerosHumanos) != 1 || c.NumerosHumanos[0] != "5527999991111" || !c.ehHumano("552799991111") {
		t.Errorf("número antigo não virou lista: %+v", c.NumerosHumanos)
	}
	c = Config{NumerosHumanos: []string{"5527999991111", "552799991111", "5527999992222", "123"}}
	c.normalizar()
	if len(c.NumerosHumanos) != 2 || c.NumeroHumano != "5527999991111" || !c.ehHumano("5527999992222") || c.ehHumano("5527999993333") {
		t.Errorf("lista de números mal normalizada: %+v", c.NumerosHumanos)
	}
}

func TestRelatorioEHistorico(t *testing.T) {
	agora := time.Date(2026, 11, 11, 15, 0, 0, 0, time.Local)
	h := func(dias int) time.Time { return agora.AddDate(0, 0, -dias) }
	leads := []*Lead{
		{ID: "a", Status: statusHumano, Classificacao: "quente", Etiquetas: []string{"vip", "orçamento"}, CriadoEm: h(1), Historico: []Msg{
			{De: "cliente", Texto: "oi", Hora: h(1)},
			{De: "atendente", Texto: "olá", Hora: h(1)},
			{De: "sistema", Tipo: "evento", Evento: "transbordo", Texto: "Transferido", Hora: h(1)},
		}},
		{ID: "b", Status: statusAtendimento, Classificacao: "morno", Etiquetas: []string{"vip"}, CriadoEm: h(20), Historico: []Msg{
			{De: "cliente", Texto: "antiga", Hora: h(20)},
			{De: "cliente", Tipo: "imagem", Hora: h(0)},
			{De: "humano", Texto: "resolvido", Autor: "Oliveira", Hora: h(0)},
			{De: "cliente", Texto: "obrigado", Hora: h(0)},
			{De: "cliente", Texto: "valeu", Hora: h(0)},
		}},
		{ID: "c", Status: statusQualificando, Classificacao: "indefinido", CriadoEm: h(40)},
	}
	agenda := []*Agendamento{{Status: "marcado", CriadoEm: h(2)}, {Status: "cancelado", CriadoEm: h(2)}, {Status: "marcado", CriadoEm: h(15)}}

	r := montarRelatorio(leads, agenda, 7, agora)
	p, a := r["periodo"].(map[string]int), r["agora"].(map[string]int)
	querP := map[string]int{"conversas": 2, "novas": 1, "mensagens_clientes": 4, "respostas_assistente": 1, "respostas_equipe": 1, "transbordos": 1, "agendamentos": 1, "cancelamentos": 1}
	for k, v := range querP {
		if p[k] != v {
			t.Errorf("periodo[%s] = %d, quero %d", k, p[k], v)
		}
	}
	querA := map[string]int{"total": 3, "com_assistente": 1, "aguardando": 1, "em_atendimento": 1, "quentes": 1, "mornos": 1, "frios": 0, "indefinidos": 1}
	for k, v := range querA {
		if a[k] != v {
			t.Errorf("agora[%s] = %d, quero %d", k, a[k], v)
		}
	}
	if e := r["por_etiqueta"].([]contagem); len(e) != 2 || e[0].Nome != "vip" || e[0].Total != 2 {
		t.Errorf("etiquetas do relatório: %+v", e)
	}
	dias := r["por_dia"].([]diaRelatorio)
	if len(dias) != 7 || dias[6].Data != "2026-11-11" || dias[6].Mensagens != 4 || dias[5].Novas != 1 || dias[5].Transbordos != 1 {
		t.Errorf("por dia: %+v", dias)
	}
	if eq := r["atendentes"].([]map[string]any); len(eq) != 1 || eq[0]["nome"] != "Oliveira" {
		t.Errorf("equipe: %+v", eq)
	}
	if tudo := montarRelatorio(leads, agenda, 0, agora)["periodo"].(map[string]int); tudo["mensagens_clientes"] != 5 || tudo["agendamentos"] != 2 {
		t.Errorf("relatório de tudo: %+v", tudo)
	}

	// A IA nao ve as linhas internas, e midia sem legenda vira uma palavra entre colchetes.
	entrada := montarEntrada(leads[1].Historico[:3], []string{"obrigado"}, leads[1], nil)
	if strings.Contains(montarEntrada(leads[0].Historico, []string{"x"}, nil, nil), "Transferido") {
		t.Errorf("evento interno foi parar na conversa da IA")
	}
	if !strings.Contains(entrada, "Cliente: [imagem]") || !strings.Contains(entrada, "Equipe: resolvido") {
		t.Errorf("entrada da IA sem a mídia ou sem a resposta da equipe:\n%s", entrada)
	}

	// Previa e nao lidas na lista de conversas.
	app := novoApp()
	v := app.visaoLead(leads[1], agora)
	if v.NaoLidas != 2 || v.Previa != "valeu" || v.PreviaTipo != "texto" || v.TemFoto {
		t.Errorf("visão do lead: nao_lidas=%d previa=%q tipo=%q foto=%v", v.NaoLidas, v.Previa, v.PreviaTipo, v.TemFoto)
	}
	if v := app.visaoLead(leads[0], agora); v.NaoLidas != 0 || v.Previa != "olá" || v.PreviaDe != "atendente" {
		t.Errorf("a prévia devia pular o evento interno: %+v", v)
	}
}
