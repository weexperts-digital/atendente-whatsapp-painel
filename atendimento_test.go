package main

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestEtiquetasDaIA(t *testing.T) {
	l := &Lead{Etiquetas: []string{"VIP"}}
	emUso := []string{"Orçamento Pedido", "VIP"}

	l.etiquetasDaIA([]string{"orcamento pedido", " Lavagem. ", "vip"}, emUso)
	if got := strings.Join(l.Etiquetas, "|"); got != "VIP|Orçamento Pedido|lavagem" {
		t.Fatalf("devia reaproveitar a grafia do painel, limpar e não repetir: %s", got)
	}
	if got := strings.Join(l.EtiquetasIA, "|"); got != "Orçamento Pedido|lavagem" {
		t.Errorf("só as novas contam como criadas pela IA: %s", got)
	}

	// O que não serve como etiqueta: temperatura, dado pessoal, frase longa.
	antes := len(l.Etiquetas)
	l.etiquetasDaIA([]string{"quente", "27 99999-1234", "joao@exemplo.com"}, emUso)
	l.etiquetasDaIA([]string{"cliente que perguntou sobre muitas coisas diferentes", "", "Lead Quente"}, emUso)
	if len(l.Etiquetas) != antes {
		t.Errorf("etiqueta ruim entrou: %v", l.Etiquetas)
	}

	// No máximo 3 por resposta.
	l.etiquetasDaIA([]string{"a1", "a2", "a3", "a4"}, nil)
	if temEtiqueta(l.Etiquetas, "a4") || !temEtiqueta(l.Etiquetas, "a3") {
		t.Errorf("devia aceitar só 3 por vez: %v", l.Etiquetas)
	}

	// Uma pessoa tira uma etiqueta da IA: ela não volta. Se a pessoa puser de novo, volta a valer.
	semLavagem := semEtiqueta(l.Etiquetas, "lavagem")
	velhas := l.Etiquetas
	l.Etiquetas = semLavagem
	l.etiquetasEditadas(velhas)
	l.etiquetasDaIA([]string{"Lavagem"}, nil)
	if temEtiqueta(l.Etiquetas, "lavagem") || temEtiqueta(l.EtiquetasIA, "lavagem") || !temEtiqueta(l.EtiquetasFora, "lavagem") {
		t.Errorf("etiqueta tirada por uma pessoa não pode voltar: %v / fora %v", l.Etiquetas, l.EtiquetasFora)
	}
	velhas = l.Etiquetas
	l.Etiquetas = append(append([]string{}, l.Etiquetas...), "lavagem")
	l.etiquetasEditadas(velhas)
	if temEtiqueta(l.EtiquetasFora, "lavagem") || temEtiqueta(l.EtiquetasIA, "lavagem") {
		t.Errorf("etiqueta posta de volta por uma pessoa deixa de estar vetada e não é da IA: fora %v, ia %v", l.EtiquetasFora, l.EtiquetasIA)
	}

	// Passou do limite da IA: sai a mais antiga da própria IA, nunca a de uma pessoa.
	for i := 0; i < 10; i++ {
		l.etiquetasDaIA([]string{fmt.Sprintf("assunto %d", i)}, nil)
	}
	if len(l.EtiquetasIA) != maxEtiquetasIA || !temEtiqueta(l.Etiquetas, "VIP") || !temEtiqueta(l.Etiquetas, "lavagem") {
		t.Errorf("limite da IA: %d etiquetas da IA, todas: %v", len(l.EtiquetasIA), l.Etiquetas)
	}
	if !temEtiqueta(l.Etiquetas, "assunto 9") || temEtiqueta(l.Etiquetas, "assunto 0") {
		t.Errorf("devia ficar com as mais novas: %v", l.Etiquetas)
	}

	// Conversa cheia de etiquetas postas por pessoas: a IA não mexe.
	cheia := &Lead{}
	for i := 0; i < 12; i++ {
		cheia.Etiquetas = append(cheia.Etiquetas, fmt.Sprintf("manual %d", i))
	}
	cheia.etiquetasDaIA([]string{"nova"}, nil)
	if len(cheia.Etiquetas) != 12 || temEtiqueta(cheia.Etiquetas, "nova") {
		t.Errorf("a IA não pode tirar etiqueta de pessoa para pôr a dela: %v", cheia.Etiquetas)
	}
}

func TestEtiquetasDaIAFicamGuardadas(t *testing.T) {
	b := bancoDeTeste(t)
	l := &Lead{ID: "5527999990001@s.whatsapp.net", Telefone: "5527999990001", CriadoEm: time.Now(), UltimaMsg: time.Now()}
	l.etiquetasDaIA([]string{"orçamento pedido"}, nil)
	velhas := l.Etiquetas
	l.Etiquetas = []string{}
	l.etiquetasEditadas(velhas)
	l.etiquetasDaIA([]string{"quer agendar"}, nil)
	if err := b.gravarLeads(map[string]*Lead{l.ID: l}); err != nil {
		t.Fatal(err)
	}
	lidos, err := b.lerLeads(maxHistorico)
	if err != nil {
		t.Fatal(err)
	}
	var volta *Lead
	for _, x := range lidos {
		if x.ID == l.ID {
			volta = x
		}
	}
	if volta == nil || strings.Join(volta.Etiquetas, "|") != "quer agendar" || strings.Join(volta.EtiquetasIA, "|") != "quer agendar" || strings.Join(volta.EtiquetasFora, "|") != "orçamento pedido" {
		t.Errorf("etiquetas depois de religar: %+v", volta)
	}
}

func TestEsquemaDaSaidaTemEtiquetas(t *testing.T) {
	var esquema struct {
		Properties struct {
			Lead struct {
				Properties map[string]any `json:"properties"`
				Required   []string       `json:"required"`
			} `json:"lead"`
		} `json:"properties"`
	}
	if err := json.Unmarshal([]byte(esquemaSaida), &esquema); err != nil {
		t.Fatal(err)
	}
	if _, tem := esquema.Properties.Lead.Properties["etiquetas"]; !tem || !strings.Contains(strings.Join(esquema.Properties.Lead.Required, ","), "etiquetas") {
		t.Errorf("o esquema da saída precisa pedir lead.etiquetas")
	}
	s, err := lerSaida([]byte(`{"resposta":"oi","transbordo":false,"motivo":"","lead":{"nome":"","necessidade":"","orcamento":"","decisor":"","prazo":"","pontuacao":10,"classificacao":"frio","etapa":"","resumo":"","etiquetas":["troca de óleo"]},"agendamento":{"acao":"nenhuma","servico":"","data":"","hora":""}}`))
	if err != nil || len(s.Lead.Etiquetas) != 1 || s.Lead.Etiquetas[0] != "troca de óleo" {
		t.Errorf("saída com etiquetas: %+v, %v", s.Lead, err)
	}
}

// conversaLonga monta uma conversa em que a IA atendeu, a equipe assumiu, conversou
// bastante e devolveu.
func conversaLonga(comIA, comEquipe int) []Msg {
	var h []Msg
	for i := 0; i < comIA; i++ {
		h = append(h, Msg{De: "cliente", Texto: fmt.Sprintf("pergunta %d para a IA", i)})
		h = append(h, Msg{De: "atendente", Texto: fmt.Sprintf("resposta %d da IA", i)})
	}
	h = append(h, Msg{De: "sistema", Tipo: "evento", Evento: "transbordo", Texto: "Transferido para a equipe: lead quente"})
	h = append(h, Msg{De: "sistema", Tipo: "evento", Evento: "assumiu", Texto: "Oliveira iniciou o atendimento"})
	for i := 0; i < comEquipe; i++ {
		h = append(h, Msg{De: "cliente", Texto: fmt.Sprintf("pergunta %d para a equipe", i)})
		h = append(h, Msg{De: "humano", Autor: "Oliveira", Texto: fmt.Sprintf("combinado %d com a equipe", i)})
	}
	h = append(h, Msg{De: "sistema", Tipo: "evento", Evento: "devolveu", Texto: "Oliveira devolveu a conversa ao assistente"})
	return h
}

func TestIAReceTudoOQueAEquipeConversou(t *testing.T) {
	l := &Lead{Retomado: true, Etiquetas: []string{"frota"}}
	h := conversaLonga(20, 20) // 40 mensagens com a IA, depois 40 com a equipe
	entrada := montarEntrada(h, []string{"voltei, e agora?"}, l, []string{"frota", "pós-venda"})

	for i := 0; i < 20; i++ {
		if !strings.Contains(entrada, fmt.Sprintf("Equipe: combinado %d com a equipe\n", i)) || !strings.Contains(entrada, fmt.Sprintf("Cliente: pergunta %d para a equipe\n", i)) {
			t.Fatalf("a IA precisa ler o atendimento humano inteiro, faltou a troca %d", i)
		}
	}
	for _, marca := range []string{"[O assistente passou a conversa para a equipe]", "[A equipe assumiu a conversa]", "[A equipe devolveu a conversa para o assistente]"} {
		if !strings.Contains(entrada, marca) {
			t.Errorf("faltou a marca %q", marca)
		}
	}
	if strings.Contains(entrada, "Oliveira") || strings.Contains(entrada, "Transferido") {
		t.Errorf("nome de quem atendeu ou texto interno foi para a IA:\n%s", entrada)
	}
	// Ordem: equipe assume, conversa, devolve, e só então a mensagem nova.
	assumiu := strings.Index(entrada, "[A equipe assumiu a conversa]")
	meio := strings.Index(entrada, "Equipe: combinado 10 com a equipe")
	devolveu := strings.Index(entrada, "[A equipe devolveu a conversa para o assistente]")
	nova := strings.Index(entrada, "Cliente: voltei, e agora?")
	if !(assumiu < meio && meio < devolveu && devolveu < nova) {
		t.Errorf("as linhas saíram fora de ordem: %d %d %d %d", assumiu, meio, devolveu, nova)
	}
	if !strings.Contains(entrada, "continue de onde a equipe parou") {
		t.Errorf("faltou a orientação de continuar de onde a equipe parou")
	}
	if !strings.Contains(entrada, `"etiquetas":["frota"]`) || !strings.Contains(entrada, "Etiquetas já usadas no painel: frota, pós-venda\n") {
		t.Errorf("faltaram as etiquetas da conversa ou as do painel:\n%s", entrada[len(entrada)-600:])
	}
	if n := strings.Count(entrada, "\nAtendente: ") + strings.Count(entrada, "\nEquipe: ") + strings.Count(entrada, "Cliente: "); n != janelaIAEquipe+1 {
		t.Errorf("com a equipe na conversa a IA lê %d mensagens mais a nova, leu %d", janelaIAEquipe, n)
	}
}

func TestIAReceJanelaNormalSemEquipe(t *testing.T) {
	var h []Msg
	for i := 0; i < 50; i++ {
		h = append(h, Msg{De: "cliente", Texto: fmt.Sprintf("pergunta %d", i)}, Msg{De: "atendente", Texto: fmt.Sprintf("resposta %d", i)})
	}
	h = append(h, Msg{De: "sistema", Tipo: "evento", Evento: "agendou", Texto: "Horário marcado pelo assistente: Corte"})
	entrada := montarEntrada(h, []string{"oi de novo"}, &Lead{}, nil)
	if strings.Contains(entrada, "pergunta 34\n") || !strings.Contains(entrada, "pergunta 35\n") || !strings.Contains(entrada, "resposta 49\n") {
		t.Errorf("sem equipe, a IA lê as últimas %d mensagens", janelaIA)
	}
	if strings.Contains(entrada, "Horário marcado") || strings.Contains(entrada, "[") && strings.Contains(entrada, "equipe]") {
		t.Errorf("linha interna foi para a IA")
	}
	if strings.Contains(entrada, "Etiquetas já usadas") || strings.Contains(entrada, "Observação:") {
		t.Errorf("sem etiquetas no painel e sem atendimento humano, essas linhas não aparecem")
	}

	// Mensagem enorme é cortada para não estourar a conversa.
	grande := montarEntrada([]Msg{{De: "cliente", Texto: strings.Repeat("a", 5000)}}, []string{"x"}, nil, nil)
	if len(grande) > 1500 {
		t.Errorf("mensagem anterior enorme devia ser cortada, a entrada ficou com %d letras", len(grande))
	}
}
