package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestEtiquetasDaIA(t *testing.T) {
	l := &Lead{Etiquetas: []string{"VIP"}}
	emUso := []string{"Orçamento Pedido", "VIP", "Corte"}

	// Primeira resposta: a IA usa a grafia do painel, limpa e não repete.
	postas, tiradas := l.etiquetasDaIA([]string{"orcamento pedido", " corte. ", "vip", "novo contato"}, emUso)
	if got := strings.Join(l.Etiquetas, "|"); got != "VIP|Orçamento Pedido|Corte|novo contato" {
		t.Fatalf("devia reaproveitar a grafia do painel, limpar e não repetir: %s", got)
	}
	if strings.Join(l.EtiquetasIA, "|") != "Orçamento Pedido|Corte|novo contato" || len(postas) != 3 || len(tiradas) != 0 {
		t.Errorf("só as novas são da IA: %v, postas %v, tiradas %v", l.EtiquetasIA, postas, tiradas)
	}

	// A pessoa avançou: a IA troca a etiqueta de momento. A antiga sai, a da pessoa (VIP) fica.
	postas, tiradas = l.etiquetasDaIA([]string{"corte", "agendado"}, emUso)
	if got := strings.Join(l.Etiquetas, "|"); got != "VIP|Corte|agendado" {
		t.Fatalf("a IA devia trocar as etiquetas dela: %s", got)
	}
	if strings.Join(postas, "|") != "agendado" || strings.Join(tiradas, "|") != "Orçamento Pedido|novo contato" {
		t.Errorf("postas %v, tiradas %v", postas, tiradas)
	}
	if frase := fraseDeEtiquetas("a IA", postas, tiradas); frase != "Etiquetas: a IA pôs agendado e tirou Orçamento Pedido, novo contato" {
		t.Errorf("frase do histórico: %q", frase)
	}

	// Lista vazia ou só com coisa ruim não apaga nada.
	antes := strings.Join(l.Etiquetas, "|")
	for _, ruim := range [][]string{nil, {}, {"quente", "27 99999-1234", "joao@exemplo.com"}, {"cliente que perguntou sobre muitas coisas diferentes", "", "Lead Quente"}} {
		if p, tr := l.etiquetasDaIA(ruim, emUso); len(p)+len(tr) != 0 || strings.Join(l.Etiquetas, "|") != antes {
			t.Errorf("resposta sem etiqueta boa não pode mexer em nada: %v -> %v", ruim, l.Etiquetas)
		}
	}

	// Uma pessoa tira uma etiqueta da IA: ela não volta. Se a pessoa puser de novo, volta a valer e passa a ser da pessoa.
	velhas := l.Etiquetas
	l.Etiquetas = semEtiqueta(l.Etiquetas, "agendado")
	l.etiquetasEditadas(velhas)
	l.etiquetasDaIA([]string{"corte", "Agendado"}, emUso)
	if temEtiqueta(l.Etiquetas, "agendado") || !temEtiqueta(l.EtiquetasFora, "agendado") {
		t.Errorf("etiqueta tirada por uma pessoa não pode voltar: %v / fora %v", l.Etiquetas, l.EtiquetasFora)
	}
	velhas = l.Etiquetas
	l.Etiquetas = append(append([]string{}, l.Etiquetas...), "agendado")
	l.etiquetasEditadas(velhas)
	if temEtiqueta(l.EtiquetasFora, "agendado") || temEtiqueta(l.EtiquetasIA, "agendado") {
		t.Errorf("posta de volta por uma pessoa: fora %v, ia %v", l.EtiquetasFora, l.EtiquetasIA)
	}
	// E a IA não tira etiqueta de pessoa, mesmo deixando fora da lista.
	l.etiquetasDaIA([]string{"cliente"}, emUso)
	if !temEtiqueta(l.Etiquetas, "VIP") || !temEtiqueta(l.Etiquetas, "agendado") || !temEtiqueta(l.Etiquetas, "cliente") || temEtiqueta(l.Etiquetas, "Corte") {
		t.Errorf("a IA só mexe nas dela: %v", l.Etiquetas)
	}

	// No máximo 5 etiquetas da IA por conversa, e nunca mais de 12 no total.
	muitas := &Lead{}
	muitas.etiquetasDaIA([]string{"a1", "a2", "a3", "a4", "a5", "a6", "a7"}, nil)
	if len(muitas.EtiquetasIA) != maxEtiquetasIA {
		t.Errorf("limite da IA: %v", muitas.EtiquetasIA)
	}
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

func TestEtiquetasUnicasNoPainel(t *testing.T) {
	a := novoApp()
	a.leads["a"] = &Lead{ID: "a", Etiquetas: []string{"Corte", "cliente"}, EtiquetasIA: []string{"Corte"}}
	a.leads["b"] = &Lead{ID: "b", Etiquetas: []string{"corte de cabelo", "Cliente"}, EtiquetasIA: []string{"corte de cabelo"}}
	a.leads["c"] = &Lead{ID: "c", Etiquetas: []string{"corte", "corte de cabelo"}}

	// O catálogo não repete: maiúsculas e acentos não criam etiqueta nova.
	cat := a.catalogoDeEtiquetas()
	if len(cat) != 3 || chaveTexto(cat[0].Nome) != "cliente" && chaveTexto(cat[0].Nome) != "corte" {
		t.Fatalf("catálogo: %+v", cat)
	}
	conta := map[string]etiquetaDoPainel{}
	for _, e := range cat {
		conta[chaveTexto(e.Nome)] = e
	}
	if conta["corte"].Contatos != 2 || conta["corte"].DaIA != 1 || conta["cliente"].Contatos != 2 || conta["corte de cabelo"].Contatos != 2 {
		t.Errorf("contagem do catálogo: %+v", cat)
	}

	// Juntar "corte de cabelo" em "corte": quem tinha as duas fica com uma só.
	w := postar(a.rotaRenomearEtiqueta, `{"de":"corte de cabelo","para":"CORTE"}`)
	if w.Code != http.StatusOK {
		t.Fatalf("juntar etiquetas: %d %s", w.Code, w.Body.String())
	}
	for id, quer := range map[string]string{"a": "Corte|cliente", "b": "Corte|Cliente", "c": "corte"} {
		if got := strings.Join(a.leads[id].Etiquetas, "|"); chaveTexto(got) != chaveTexto(quer) {
			t.Errorf("contato %s depois de juntar: %s", id, got)
		}
	}
	if len(a.leads["c"].Etiquetas) != 1 || !temEtiqueta(a.leads["b"].EtiquetasIA, "corte") {
		t.Errorf("juntar não pode deixar repetida e mantém o que era da IA: c=%v b.ia=%v", a.leads["c"].Etiquetas, a.leads["b"].EtiquetasIA)
	}
	if len(a.catalogoDeEtiquetas()) != 2 {
		t.Errorf("depois de juntar sobram 2 etiquetas: %+v", a.catalogoDeEtiquetas())
	}

	// Apagar tira de todos os contatos e a IA não repõe nesses contatos.
	if w := postar(a.rotaExcluirEtiqueta, `{"nome":"cliente"}`); w.Code != http.StatusOK {
		t.Fatalf("apagar etiqueta: %d %s", w.Code, w.Body.String())
	}
	if temEtiqueta(a.leads["a"].Etiquetas, "cliente") || !temEtiqueta(a.leads["a"].EtiquetasFora, "cliente") {
		t.Errorf("apagar: %v fora %v", a.leads["a"].Etiquetas, a.leads["a"].EtiquetasFora)
	}
	if w := postar(a.rotaExcluirEtiqueta, `{"nome":"não existe"}`); w.Code != http.StatusNotFound {
		t.Errorf("apagar etiqueta que não existe devia dar 404, deu %d", w.Code)
	}
	if w := postar(a.rotaRenomearEtiqueta, `{"de":"corte","para":"  "}`); w.Code != http.StatusBadRequest {
		t.Errorf("renomear para vazio devia dar 400, deu %d", w.Code)
	}

	// Editar as etiquetas de um contato pelo painel usa a grafia que já existe e anota no histórico.
	a.leads["d"] = &Lead{ID: "d", Telefone: "5527999990004"}
	w = postar(a.rotaEtiquetas, `{"id":"d","etiquetas":["CORTE","novidade"]}`)
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"Corte"`) {
		t.Fatalf("etiquetas do contato: %d %s", w.Code, w.Body.String())
	}
	d := a.leads["d"]
	ultimo := d.Historico[len(d.Historico)-1]
	if ultimo.De != "sistema" || ultimo.Evento != "etiqueta" || !strings.Contains(ultimo.Texto, "pôs Corte, novidade") {
		t.Errorf("a mudança de etiqueta devia entrar no histórico: %+v", ultimo)
	}
	// E esse registro interno nunca vai para a IA.
	if entrada := montarEntrada(d.Historico, []string{"oi"}, d, nil); strings.Contains(entrada, "pôs Corte") {
		t.Errorf("registro de etiqueta foi para a IA:\n%s", entrada)
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

func TestUnificarEtiquetasAntigas(t *testing.T) {
	a := novoApp()
	a.leads["a"] = &Lead{ID: "a", Etiquetas: []string{"cliente", "Orçamento"}}
	a.leads["b"] = &Lead{ID: "b", Etiquetas: []string{"Cliente", "orcamento", "ORÇAMENTO"}, EtiquetasIA: []string{"orcamento"}}
	a.leads["c"] = &Lead{ID: "c", Etiquetas: []string{"Cliente", "Orçamento"}}
	if n := a.unificarEtiquetas(); n != 2 {
		t.Errorf("deviam mudar 2 contatos, mudaram %d", n)
	}
	for id, l := range a.leads {
		if strings.Join(l.Etiquetas, "|") != "Cliente|Orçamento" {
			t.Errorf("contato %s: %v", id, l.Etiquetas)
		}
	}
	if strings.Join(a.leads["b"].EtiquetasIA, "|") != "Orçamento" {
		t.Errorf("as etiquetas da IA acompanham a grafia: %v", a.leads["b"].EtiquetasIA)
	}
	if em := a.etiquetasEmUso(); len(em) != 2 {
		t.Errorf("a lista em uso não pode repetir: %v", em)
	}
	if n := a.unificarEtiquetas(); n != 0 {
		t.Errorf("segunda passada não muda nada, mudou %d", n)
	}
}
