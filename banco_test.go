package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func bancoDeTeste(t *testing.T) *Banco {
	t.Helper()
	b, err := abrirBanco(filepath.Join(t.TempDir(), "teste.db"))
	if err != nil {
		t.Fatalf("abrindo o banco: %v", err)
	}
	t.Cleanup(b.fechar)
	return b
}

func contar(t *testing.T, b *Banco, consulta string, args ...any) int {
	t.Helper()
	var n int
	if err := b.db.QueryRow(consulta, args...).Scan(&n); err != nil {
		t.Fatalf("%s: %v", consulta, err)
	}
	return n
}

func TestBancoGuardaContatosEMensagens(t *testing.T) {
	caminho := filepath.Join(t.TempDir(), "teste.db")
	b, err := abrirBanco(caminho)
	if err != nil {
		t.Fatal(err)
	}
	agora := time.Now()
	ana := &Lead{ID: "5527999990001@s.whatsapp.net", Telefone: "5527999990001", Nome: "Ana", Classificacao: "quente", Pontuacao: 80, Status: statusHumano, Etiquetas: []string{"vip"}, CriadoEm: agora, UltimaMsg: agora}
	ana.anexar(Msg{De: "cliente", Texto: "Oi", Hora: agora})
	ana.anexar(Msg{De: "cliente", Tipo: "imagem", ID: "ABC", Hora: agora})
	ana.evento("transbordo", "Pediu uma pessoa")
	bia := &Lead{ID: "5527999990002@s.whatsapp.net", Telefone: "5527999990002", NomeWhats: "Bia", Classificacao: "indefinido", Status: statusQualificando, CriadoEm: agora, UltimaMsg: agora}
	bia.anexar(Msg{De: "cliente", Texto: "Bom dia", Hora: agora})
	leads := map[string]*Lead{ana.ID: ana, bia.ID: bia}

	if err := b.gravarLeads(leads); err != nil {
		t.Fatalf("gravando: %v", err)
	}
	for _, m := range ana.Historico {
		if m.N == 0 {
			t.Fatalf("mensagem ficou sem número do banco: %+v", m)
		}
	}
	if n := contar(t, b, `SELECT COUNT(*) FROM mensagens`); n != 4 {
		t.Fatalf("esperava 4 mensagens gravadas, achei %d", n)
	}

	// Gravar de novo sem mudar nada nao pode duplicar.
	if err := b.gravarLeads(leads); err != nil {
		t.Fatal(err)
	}
	if n := contar(t, b, `SELECT COUNT(*) FROM mensagens`); n != 4 {
		t.Fatalf("gravar duas vezes duplicou mensagens: %d", n)
	}

	// A midia chega depois: a mesma mensagem e atualizada, nao criada de novo.
	ana.Historico[1].Midia = "f0123456789.jpg"
	ana.Historico[1].Mime = "image/jpeg"
	ana.Historico[1].Tamanho = 1234
	ana.Etiquetas = append(ana.Etiquetas, "orçamento")
	if err := b.gravarLeads(leads); err != nil {
		t.Fatal(err)
	}
	b.fechar()

	// Abre de novo, como se o programa tivesse sido religado.
	b, err = abrirBanco(caminho)
	if err != nil {
		t.Fatal(err)
	}
	defer b.fechar()
	lidos, err := b.lerLeads(maxHistorico)
	if err != nil {
		t.Fatalf("lendo: %v", err)
	}
	porID := map[string]*Lead{}
	for _, l := range lidos {
		porID[l.ID] = l
	}
	a2 := porID[ana.ID]
	if a2 == nil || len(porID) != 2 {
		t.Fatalf("esperava 2 contatos, achei %d", len(porID))
	}
	if a2.Nome != "Ana" || a2.Pontuacao != 80 || a2.Status != statusHumano || len(a2.Etiquetas) != 2 || a2.TotalMsgs != 2 {
		t.Errorf("contato voltou diferente: %+v", a2)
	}
	if len(a2.Historico) != 3 || a2.Historico[0].Texto != "Oi" || a2.Historico[1].Midia != "f0123456789.jpg" || a2.Historico[1].Tamanho != 1234 || a2.Historico[2].Evento != "transbordo" {
		t.Errorf("mensagens voltaram diferentes: %+v", a2.Historico)
	}
	if !a2.Historico[0].Hora.Equal(agora) {
		t.Errorf("hora da mensagem mudou: %v != %v", a2.Historico[0].Hora, agora)
	}
	if n := contar(t, b, `SELECT COUNT(*) FROM contatos WHERE temperatura = 'quente' AND situacao = 'fila'`); n != 1 {
		t.Errorf("colunas de busca não foram preenchidas")
	}

	// Depois de ler, gravar sem mudancas nao reescreve nem duplica.
	lidosMapa := map[string]*Lead{a2.ID: a2, bia.ID: porID[bia.ID]}
	if err := b.gravarLeads(lidosMapa); err != nil {
		t.Fatal(err)
	}
	if n := contar(t, b, `SELECT COUNT(*) FROM mensagens`); n != 4 {
		t.Fatalf("religar duplicou mensagens: %d", n)
	}

	// Apagar o contato leva as mensagens dele junto.
	delete(lidosMapa, bia.ID)
	if err := b.gravarLeads(lidosMapa); err != nil {
		t.Fatal(err)
	}
	if n := contar(t, b, `SELECT COUNT(*) FROM contatos`); n != 1 {
		t.Errorf("contato apagado continuou no banco")
	}
	if n := contar(t, b, `SELECT COUNT(*) FROM mensagens WHERE contato_id = ?`, bia.ID); n != 0 {
		t.Errorf("mensagens do contato apagado continuaram no banco: %d", n)
	}
}

func TestBancoGuardaConversaInteira(t *testing.T) {
	b := bancoDeTeste(t)
	l := &Lead{ID: "5527999990003@s.whatsapp.net", Status: statusQualificando}
	leads := map[string]*Lead{l.ID: l}
	total := maxHistorico + 45
	for i := 1; i <= total; i++ {
		l.anexar(Msg{De: "cliente", Texto: fmt.Sprintf("mensagem %d", i), Hora: time.Now()})
		if i%7 == 0 || i == total { // grava de vez em quando, como no uso real
			if err := b.gravarLeads(leads); err != nil {
				t.Fatal(err)
			}
		}
	}
	if len(l.Historico) != maxHistorico {
		t.Fatalf("a memória devia guardar só as últimas %d, tem %d", maxHistorico, len(l.Historico))
	}
	if n := contar(t, b, `SELECT COUNT(*) FROM mensagens`); n != total {
		t.Fatalf("o banco devia guardar todas as %d mensagens, tem %d", total, n)
	}
	if l.TotalMsgs != total {
		t.Errorf("total de mensagens do contato = %d, esperava %d", l.TotalMsgs, total)
	}

	lidos, err := b.lerLeads(maxHistorico)
	if err != nil || len(lidos) != 1 {
		t.Fatalf("lendo: %v (%d contatos)", err, len(lidos))
	}
	h := lidos[0].Historico
	if len(h) != maxHistorico || h[0].Texto != "mensagem 46" || h[len(h)-1].Texto != fmt.Sprintf("mensagem %d", total) {
		t.Fatalf("esperava as últimas %d mensagens em ordem, veio de %q a %q (%d)", maxHistorico, h[0].Texto, h[len(h)-1].Texto, len(h))
	}
	if !b.temAntes(l.ID, h[0].N) {
		t.Errorf("devia avisar que existem mensagens mais antigas")
	}
	antigas, temMais, err := b.mensagensAntes(l.ID, h[0].N, 30)
	if err != nil || len(antigas) != 30 || !temMais || antigas[0].Texto != "mensagem 16" || antigas[29].Texto != "mensagem 45" {
		t.Fatalf("mensagens antigas erradas: %v, %d, mais=%v", err, len(antigas), temMais)
	}
	resto, temMais, _ := b.mensagensAntes(l.ID, antigas[0].N, 30)
	if len(resto) != 15 || temMais || resto[0].Texto != "mensagem 1" {
		t.Fatalf("começo da conversa errado: %d mensagens, mais=%v", len(resto), temMais)
	}
}

func TestBancoAgendaEConfig(t *testing.T) {
	b := bancoDeTeste(t)
	if _, existe, err := b.lerConfig(); err != nil || existe {
		t.Fatalf("banco novo não devia ter dados do negócio: %v %v", existe, err)
	}
	cfg := Config{Negocio: "Barbearia Teste", Salvo: true, NumerosHumanos: listaNumeros{"5527999990000"}}
	cfg.normalizar()
	if err := b.gravarConfig(cfg); err != nil {
		t.Fatal(err)
	}
	lida, existe, err := b.lerConfig()
	if err != nil || !existe || lida.Negocio != "Barbearia Teste" || !lida.Salvo || len(lida.NumerosHumanos) != 1 {
		t.Fatalf("dados do negócio voltaram diferentes: %+v (%v)", lida, err)
	}

	inicio := time.Now().Add(48 * time.Hour).Truncate(time.Minute)
	ags := []*Agendamento{
		{ID: "a1", LeadID: "x", Nome: "Ana", Servico: "Corte", Inicio: inicio, DuracaoMin: 30, Origem: "assistente", Status: "marcado", CriadoEm: time.Now()},
		{ID: "a2", Nome: "Bia", Servico: "Barba", Inicio: inicio.Add(time.Hour), DuracaoMin: 20, Origem: "painel", Status: "marcado", CriadoEm: time.Now()},
	}
	if err := b.gravarAgenda(ags); err != nil {
		t.Fatal(err)
	}
	ags[0].Status = "cancelado"
	if err := b.gravarAgenda(ags[:1]); err != nil { // a2 saiu da lista
		t.Fatal(err)
	}
	lidos, err := b.lerAgenda()
	if err != nil || len(lidos) != 1 || lidos[0].ID != "a1" || lidos[0].Status != "cancelado" || !lidos[0].Inicio.Equal(inicio) || lidos[0].DuracaoMin != 30 {
		t.Fatalf("agenda voltou diferente: %+v (%v)", lidos, err)
	}
}

// Quem vem da versao anterior tem os dados em arquivos .json. Na primeira vez o
// programa leva tudo para o banco e guarda os arquivos com o final .migrado.
func TestTrazerArquivosAntigos(t *testing.T) {
	pastaDeTeste(t)
	if err := os.MkdirAll(pastaDados, 0o700); err != nil {
		t.Fatal(err)
	}
	agora := time.Now()
	leads := []*Lead{{ID: "5527999990001@s.whatsapp.net", Telefone: "5527999990001", Nome: "Ana", Classificacao: "morno", Status: statusQualificado, CriadoEm: agora, UltimaMsg: agora,
		Historico: []Msg{{De: "cliente", Texto: "Oi", Hora: agora}, {De: "atendente", Texto: "Olá!", Hora: agora}, {De: "sistema", Tipo: "evento", Evento: "agendou", Texto: "Marcou", Hora: agora}}}}
	gravar := func(arq string, v any) {
		j, _ := json.Marshal(v)
		if err := os.WriteFile(arq, j, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	gravar(arqConfig, map[string]any{"negocio": "Barbearia Teste", "salvo": true, "ligado": true, "numero_humano": "5527999990000"})
	gravar(arqLeads, leads)
	gravar(arqAgenda, []*Agendamento{{ID: "a1", LeadID: leads[0].ID, Nome: "Ana", Servico: "Corte", Inicio: agora.Add(24 * time.Hour), DuracaoMin: 30, Status: "marcado"}})

	a := novoApp()
	a.carregar()
	if a.cfg.Negocio != "Barbearia Teste" || !a.cfg.Ligado || len(a.cfg.NumerosHumanos) != 1 || len(a.leads) != 1 || len(a.agenda) != 1 {
		t.Fatalf("a primeira carga não trouxe os arquivos: %+v, %d contatos, %d horários", a.cfg.Negocio, len(a.leads), len(a.agenda))
	}
	for _, arq := range []string{arqConfig, arqLeads, arqAgenda} {
		if existeArquivo(arq) || !existeArquivo(arq+".migrado") {
			t.Errorf("%s devia ter virado %s.migrado", arq, arq)
		}
	}
	a.banco.fechar()

	b := novoApp()
	b.carregar()
	defer b.banco.fechar()
	l := b.leads[leads[0].ID]
	if b.cfg.Negocio != "Barbearia Teste" || !b.cfg.Salvo || l == nil || len(b.agenda) != 1 {
		t.Fatalf("a segunda carga, já pelo banco, perdeu dados")
	}
	if l.Nome != "Ana" || len(l.Historico) != 3 || l.TotalMsgs != 2 || l.Historico[2].Evento != "agendou" {
		t.Errorf("contato voltou diferente do banco: %+v", l)
	}
}
