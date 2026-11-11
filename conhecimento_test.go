package main

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func docxDeTeste(t *testing.T, paragrafos ...string) []byte {
	t.Helper()
	var xml strings.Builder
	xml.WriteString(`<?xml version="1.0" encoding="UTF-8"?><w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body>`)
	for _, p := range paragrafos {
		// Duas partes por paragrafo, como o Word faz quando ha negrito no meio da frase.
		meio := len(p) / 2
		for meio < len(p) && p[meio]&0xC0 == 0x80 {
			meio++
		}
		xml.WriteString(`<w:p><w:r><w:t xml:space="preserve">` + p[:meio] + `</w:t></w:r><w:r><w:rPr><w:b/></w:rPr><w:t>` + p[meio:] + `</w:t></w:r></w:p>`)
	}
	xml.WriteString(`</w:body></w:document>`)
	var buf bytes.Buffer
	z := zip.NewWriter(&buf)
	f, err := z.Create("word/document.xml")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = f.Write([]byte(xml.String()))
	if err := z.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// pdfDeTeste monta um PDF de verdade, bem pequeno, com uma linha de texto por item.
func pdfDeTeste(linhas ...string) []byte {
	var conteudo strings.Builder
	conteudo.WriteString("BT /F1 12 Tf 72 720 Td 16 TL\n")
	for _, l := range linhas {
		conteudo.WriteString("(" + l + ") Tj T*\n")
	}
	conteudo.WriteString("ET")
	objetos := []string{
		"<< /Type /Catalog /Pages 2 0 R >>",
		"<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Contents 4 0 R /Resources << /Font << /F1 5 0 R >> >> >>",
		fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", conteudo.Len(), conteudo.String()),
		"<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica /Encoding /WinAnsiEncoding >>",
	}
	var b bytes.Buffer
	b.WriteString("%PDF-1.4\n")
	posicoes := make([]int, len(objetos))
	for i, o := range objetos {
		posicoes[i] = b.Len()
		fmt.Fprintf(&b, "%d 0 obj\n%s\nendobj\n", i+1, o)
	}
	inicioXref := b.Len()
	fmt.Fprintf(&b, "xref\n0 %d\n0000000000 65535 f \n", len(objetos)+1)
	for _, p := range posicoes {
		fmt.Fprintf(&b, "%010d 00000 n \n", p)
	}
	fmt.Fprintf(&b, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(objetos)+1, inicioXref)
	return b.Bytes()
}

func TestExtrairTexto(t *testing.T) {
	docx, err := extrairTexto("docx", docxDeTeste(t, "Tabela de preços da barbearia", "Corte de cabelo: R$ 40"))
	if err != nil {
		t.Fatalf("docx: %v", err)
	}
	if linhas := strings.Split(limparTexto(docx), "\n"); len(linhas) != 2 || linhas[0] != "Tabela de preços da barbearia" || linhas[1] != "Corte de cabelo: R$ 40" {
		t.Errorf("docx saiu diferente do esperado: %q", linhas)
	}

	pdf, err := extrairTexto("pdf", pdfDeTeste("Corte de cabelo custa 40 reais", "Barba custa 25 reais"))
	if err != nil {
		t.Fatalf("pdf: %v", err)
	}
	limpo := limparTexto(pdf)
	if !strings.Contains(limpo, "Corte de cabelo custa 40 reais") || !strings.Contains(limpo, "Barba custa 25 reais") {
		t.Errorf("pdf saiu diferente do esperado: %q", limpo)
	}

	// Texto antigo em Latin-1 ("preço" com cedilha em um byte so).
	if got := textoSimples([]byte{'p', 'r', 'e', 0xE7, 'o'}); got != "preço" {
		t.Errorf("Latin-1 virou %q", got)
	}
	for _, tipo := range []string{"doc", "exe", ""} {
		if _, err := extrairTexto(tipo, []byte("x")); err == nil {
			t.Errorf("formato %q devia ser recusado", tipo)
		}
	}
	if _, err := extrairTexto("pdf", []byte("isto não é um pdf")); err == nil {
		t.Errorf("pdf quebrado devia dar erro, não derrubar o programa")
	}
	if _, err := extrairTexto("docx", []byte("isto não é um zip")); err == nil {
		t.Errorf("docx quebrado devia dar erro")
	}
}

func TestFatiar(t *testing.T) {
	var texto strings.Builder
	for i := 0; i < 60; i++ {
		fmt.Fprintf(&texto, "Parágrafo %d fala de um assunto do negócio com algumas palavras a mais.\n\n", i)
	}
	texto.WriteString(strings.Repeat("Frase longa sem parágrafo. ", 200))
	trechos := fatiar(limparTexto(texto.String()), 900)
	if len(trechos) < 5 {
		t.Fatalf("esperava vários trechos, vieram %d", len(trechos))
	}
	for i, tr := range trechos {
		if n := len([]rune(tr)); n == 0 || n > 1400 {
			t.Errorf("trecho %d com tamanho ruim: %d", i, n)
		}
	}
	junto := strings.Join(trechos, " ")
	if !strings.Contains(junto, "Parágrafo 0 ") || !strings.Contains(junto, "Parágrafo 59 ") {
		t.Errorf("o texto perdeu pedaços ao ser fatiado")
	}
}

func TestBusca(t *testing.T) {
	pastaDeTeste(t)
	b := novaBase()
	por := func(titulo, texto string) {
		if _, err := b.adicionarTexto(titulo, texto); err != nil {
			t.Fatal(err)
		}
	}
	por("Tabela de preços", "Corte de cabelo masculino custa R$ 40. Barba custa R$ 25. A assinatura mensal custa R$ 150 e dá direito a cortes ilimitados.")
	por("Como chegar", "Estamos na Avenida Central, número 100, ao lado da padaria. Tem estacionamento gratuito na rua de trás.")
	por("Regras da casa", "Atrasos de mais de 15 minutos perdem a vez. Cancelamentos devem ser avisados com duas horas de antecedência.")

	casos := []struct{ pergunta, titulo string }{
		{"quanto custa o corte?", "Tabela de preços"},
		{"qual o preco da barba", "Tabela de preços"},
		{"onde fica? tem estacionamento?", "Como chegar"},
		{"posso cancelar em cima da hora", "Regras da casa"},
	}
	for _, c := range casos {
		achados := b.buscar(c.pergunta, 4)
		if len(achados) == 0 || achados[0].Titulo != c.titulo {
			t.Errorf("busca %q: primeiro resultado %+v, quero %q", c.pergunta, achados, c.titulo)
		}
	}
	if achados := b.buscar("oi, bom dia", 4); len(achados) != 0 {
		t.Errorf("cumprimento não devia trazer trecho nenhum: %+v", achados)
	}

	// Material desligado sai da busca e das instrucoes.
	for _, m := range b.lista() {
		if m.Titulo == "Tabela de preços" {
			b.alterar(m.ID, func(m *Material) { m.Ativo = false })
		}
	}
	if achados := b.buscar("quanto custa o corte?", 4); len(achados) > 0 && achados[0].Titulo == "Tabela de preços" {
		t.Errorf("material desligado continuou na busca")
	}

	// Guardado em disco e relido.
	outra := novaBase()
	outra.carregar()
	if outra.total() != 3 {
		t.Errorf("esperava 3 materiais relidos do disco, vieram %d", outra.total())
	}
	if _, err := b.adicionarTexto("vazio", "   "); err == nil {
		t.Errorf("material sem texto devia ser recusado")
	}
}

func TestVariaveisEConfig(t *testing.T) {
	c := Config{
		Negocio: "Barbearia Exemplo",
		Extras:  []Variavel{{"wi-fi da loja", "senha 1234"}, {"NEGOCIO", "não pode repetir uma reservada"}, {"", "sem nome"}},
	}
	c.normalizar()
	if len(c.Extras) != 1 || c.Extras[0].Nome != "WI_FI_DA_LOJA" {
		t.Fatalf("variáveis extras mal normalizadas: %+v", c.Extras)
	}
	if c.Assistente != "Ana" || c.Tom != tomPadrao || c.Modelo != modeloPadrao || c.AtrasoMin != 3 || c.AtrasoMax != 8 || c.LimiteMsgs != 6 || len(c.Agenda.Dias) != 7 {
		t.Errorf("valores padrão errados: %+v", c)
	}

	texto := trocarVariaveis("Sou {{ASSISTENTE}} de {{NEGOCIO}} em {{CIDADE}}. Wi-fi: {{WI_FI_DA_LOJA}}. Pix: {{PAGAMENTO}}.\n{{EXTRAS}}", c)
	for _, quer := range []string{"Sou Ana de Barbearia Exemplo em cidade não informada.", "Wi-fi: senha 1234.", "Pix: (não informado).", "- WI FI DA LOJA: senha 1234"} {
		if !strings.Contains(texto, quer) {
			t.Errorf("faltou %q em %q", quer, texto)
		}
	}
	if strings.Contains(texto, "{{") {
		t.Errorf("sobrou variável sem trocar: %q", texto)
	}

	// O painel manda os numeros como lista ou como texto digitado, um por linha.
	for _, corpo := range []string{`{"nunca_responder":["5527988887777","(27) 98111-2222"]}`, `{"nunca_responder":"5527988887777\n(27) 98111-2222"}`} {
		var lido Config
		if err := json.Unmarshal([]byte(corpo), &lido); err != nil {
			t.Fatalf("%s: %v", corpo, err)
		}
		lido.normalizar()
		if len(lido.NuncaResponder) != 2 || lido.NuncaResponder[1] != "27981112222" {
			t.Errorf("%s virou %v", corpo, lido.NuncaResponder)
		}
	}
}

func TestMontarSistema(t *testing.T) {
	pastaDeTeste(t)
	a := novoApp()
	a.cfg = Config{Negocio: "Barbearia Exemplo", Cidade: "Vitória/ES", Produtos: "Assinatura mensal por R$ 150"}
	a.cfg.Agenda.Ativa = true
	a.cfg.Agenda.Servicos = []Servico{{Nome: "Corte", DuracaoMin: 30, Preco: "R$ 40"}}
	a.cfg.normalizar()
	if _, err := a.base.adicionarTexto("Como chegar", "Estamos na Avenida Central, número 100, ao lado da padaria."); err != nil {
		t.Fatal(err)
	}
	t2, _ := textoPorChave("aprendizados")
	if err := salvarTexto(t2, "- Sempre ofereça a assinatura."); err != nil {
		t.Fatal(err)
	}

	sistema, trechos := a.montarSistema(a.cfg, "onde vocês ficam?", "")
	for _, quer := range []string{
		"assistente virtual de Barbearia Exemplo", "CÉREBRO DO NEGÓCIO", "Assinatura mensal por R$ 150",
		"REGRAS DE TRANSBORDO", "APRENDIZADOS", "Sempre ofereça a assinatura", "MATERIAIS DO NEGÓCIO",
		"Avenida Central", "AGENDA", "Corte (30 min), R$ 40", "Horários livres", "Hoje é ", "agendamento.acao",
	} {
		if !strings.Contains(sistema, quer) {
			t.Errorf("as instruções deviam conter %q", quer)
		}
	}
	if strings.Contains(sistema, "{{") {
		t.Errorf("sobrou variável sem trocar nas instruções")
	}
	if len(trechos) != 1 || trechos[0].Titulo != "Como chegar" {
		t.Errorf("trechos usados: %+v", trechos)
	}
}
