package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// pedirFicha chama uma rota da ficha direto no programa, sem passar pelo login.
func pedirFicha(t *testing.T, a *App, metodo, caminho string, corpo any) (int, map[string]any) {
	t.Helper()
	mux := http.NewServeMux()
	a.rotasDaFicha(mux)
	var dados []byte
	if corpo != nil {
		dados, _ = json.Marshal(corpo)
	}
	req := httptest.NewRequest(metodo, caminho, bytes.NewReader(dados))
	if corpo != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	var saida map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &saida); err != nil {
		t.Fatalf("%s %s: a resposta não é JSON: %v (%s)", metodo, caminho, err, w.Body.String())
	}
	return w.Code, saida
}

// appDaFicha monta um programa de teste com banco de verdade e um contato cheio:
// qualificacao, etiquetas, registros antigos (que ja sairam da memoria) e agenda.
func appDaFicha(t *testing.T) (*App, *Lead, string) {
	t.Helper()
	pastaDeTeste(t)
	caminho := filepath.Join(t.TempDir(), "ficha.db")
	b, err := abrirBanco(caminho)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(b.fechar)
	a := novoApp()
	a.banco = b

	agora := time.Now()
	inicio := agora.Add(-90 * 24 * time.Hour)
	l := &Lead{
		ID: "5527999990001@s.whatsapp.net", Telefone: "5527999990001", Nome: "Mariana Costa", NomeWhats: "Mari",
		Necessidade: "Site novo para a loja", Orcamento: "Até R$ 5.000", Decisor: "Ela mesma", Prazo: "Até dezembro",
		Etapa: "Proposta enviada", Resumo: "Quer um site novo e pediu proposta.",
		Pontuacao: 82, Classificacao: "quente", Status: statusQualificado,
		Etiquetas: []string{"vip", "orçamento"}, EtiquetasIA: []string{"orçamento"},
		CriadoEm: inicio, UltimaMsg: agora,
	}
	a.leads[l.ID] = l

	// Os primeiros registros acontecem no comeco da conversa...
	passo := func(i int) time.Time { return inicio.Add(time.Duration(i) * time.Hour) }
	l.anexar(Msg{De: "cliente", Texto: "Oi, quero um site", Hora: passo(0)})
	l.anexar(Msg{De: "sistema", Tipo: "evento", Evento: "transbordo", Texto: "Transferido para a equipe: pediu uma pessoa", Hora: passo(1)})
	l.anexar(Msg{De: "sistema", Tipo: "evento", Evento: "assumiu", Texto: "Oliveira iniciou o atendimento", Hora: passo(2)})
	l.anexar(Msg{De: "sistema", Tipo: "evento", Evento: "devolveu", Texto: "Oliveira devolveu a conversa ao assistente", Hora: passo(3)})
	a.salvarLeads()
	// ...e depois a conversa segue por muito mais que as 80 mensagens da memoria.
	for i := 0; i < 150; i++ {
		de := "cliente"
		if i%2 == 1 {
			de = "atendente"
		}
		l.anexar(Msg{De: de, Texto: fmt.Sprintf("mensagem %d", i), Hora: passo(4 + i)})
		if i%20 == 19 {
			a.salvarLeads()
		}
	}
	l.anexar(Msg{De: "sistema", Tipo: "evento", Evento: "agendou", Texto: "Horário marcado pelo assistente: Reunião", Hora: passo(200)})
	// Tipo que o programa ainda nao conhece: a ficha mostra do mesmo jeito, pelo texto.
	l.anexar(Msg{De: "sistema", Tipo: "evento", Evento: "etiqueta", Texto: "Etiqueta vip adicionada por Oliveira", Autor: "Oliveira", Hora: passo(201)})
	a.salvarLeads()

	a.agenda = []*Agendamento{
		{ID: "a1", LeadID: l.ID, Nome: "Mariana Costa", Telefone: l.Telefone, Servico: "Reunião", Inicio: agora.Add(48 * time.Hour), DuracaoMin: 30, Origem: "assistente", Status: "marcado", CriadoEm: agora},
		{ID: "a2", LeadID: l.ID, Nome: "Mariana Costa", Telefone: l.Telefone, Servico: "Visita", Inicio: agora.Add(-20 * 24 * time.Hour), DuracaoMin: 60, Origem: "assistente", Status: "cancelado", CriadoEm: inicio},
		// Marcado pela equipe no painel: nao tem o id da conversa, so o telefone.
		{ID: "a3", Nome: "Mariana", Telefone: "27999990001", Servico: "Entrega", Inicio: agora.Add(-5 * 24 * time.Hour), DuracaoMin: 30, Origem: "painel", Status: "marcado", CriadoEm: inicio},
		{ID: "a4", LeadID: "5527999990002@s.whatsapp.net", Nome: "Outra pessoa", Telefone: "5527999990002", Servico: "Reunião", Inicio: agora.Add(24 * time.Hour), DuracaoMin: 30, Origem: "assistente", Status: "marcado", CriadoEm: agora},
	}
	a.salvarAgenda()
	return a, l, caminho
}

func TestFichaDoContato(t *testing.T) {
	a, l, _ := appDaFicha(t)

	// Garante que o teste testa o que diz: os registros antigos sairam da memoria.
	if len(l.Historico) != maxHistorico {
		t.Fatalf("a memória devia guardar só %d mensagens, tem %d", maxHistorico, len(l.Historico))
	}
	for _, m := range l.Historico {
		if m.Evento == "transbordo" || m.Evento == "assumiu" || m.Evento == "devolveu" {
			t.Fatalf("o registro antigo %q ainda está na memória: o teste não prova a leitura do banco", m.Evento)
		}
	}

	codigo, r := pedirFicha(t, a, "GET", "/api/contato?id=5527999990001%40s.whatsapp.net", nil)
	if codigo != 200 || r["ok"] != true {
		t.Fatalf("ficha: código %d, resposta %v", codigo, r)
	}

	c, _ := r["contato"].(map[string]any)
	esperado := map[string]any{
		"id": l.ID, "exibir": "Mariana Costa", "nome": "Mariana Costa", "nome_whatsapp": "Mari",
		"telefone": "5527999990001", "telefone_bonito": "+55 27 99999-0001",
		"necessidade": "Site novo para a loja", "orcamento": "Até R$ 5.000", "decisor": "Ela mesma", "prazo": "Até dezembro",
		"etapa": "Proposta enviada", "resumo": "Quer um site novo e pediu proposta.",
		"classificacao": "quente", "status": statusQualificado, "anotacoes": "",
		"pontuacao": float64(82), "total_msgs": float64(151),
	}
	for chave, valor := range esperado {
		if c[chave] != valor {
			t.Errorf("contato.%s = %v, esperava %v", chave, c[chave], valor)
		}
	}
	if etq, _ := c["etiquetas"].([]any); len(etq) != 2 {
		t.Errorf("etiquetas vieram erradas: %v", c["etiquetas"])
	}
	if ia, _ := c["etiquetas_ia"].([]any); len(ia) != 1 || ia[0] != "orçamento" {
		t.Errorf("etiquetas da IA vieram erradas: %v", c["etiquetas_ia"])
	}
	if _, tem := c["historico"]; tem {
		t.Errorf("a ficha não devia trazer as mensagens da conversa")
	}
	if p, _ := c["proximo_agendamento"].(map[string]any); p == nil || p["servico"] != "Reunião" {
		t.Errorf("próximo agendamento veio errado: %v", c["proximo_agendamento"])
	}

	// Agendamentos: futuros, passados e cancelados, do mais novo para o mais antigo,
	// inclusive o que a equipe marcou no painel com o mesmo telefone. O de outra pessoa fica de fora.
	ags, _ := r["agendamentos"].([]any)
	var ids []string
	for _, x := range ags {
		ag, _ := x.(map[string]any)
		ids = append(ids, fmt.Sprint(ag["id"])+":"+fmt.Sprint(ag["status"]))
	}
	if got := strings.Join(ids, " "); got != "a1:marcado a3:marcado a2:cancelado" {
		t.Errorf("agendamentos = %q", got)
	}

	// Linha do tempo: vem do banco, do mais novo para o mais antigo, e termina no primeiro contato.
	hist, _ := r["historico"].([]any)
	var tipos []string
	var anterior time.Time
	for i, x := range hist {
		ev, _ := x.(map[string]any)
		tipos = append(tipos, fmt.Sprint(ev["evento"]))
		hora, err := time.Parse(time.RFC3339Nano, fmt.Sprint(ev["hora"]))
		if err != nil {
			t.Fatalf("registro %d sem hora válida: %v", i, ev)
		}
		if i > 0 && hora.After(anterior) {
			t.Errorf("registro %d (%v) está fora de ordem: devia vir do mais novo para o mais antigo", i, ev["evento"])
		}
		anterior = hora
		if strings.TrimSpace(fmt.Sprint(ev["texto"])) == "" {
			t.Errorf("registro %d veio sem texto: %v", i, ev)
		}
	}
	if got := strings.Join(tipos, " "); got != "etiqueta agendou devolveu assumiu transbordo primeiro_contato" {
		t.Errorf("linha do tempo = %q", got)
	}
	if primeiro, _ := hist[0].(map[string]any); primeiro["texto"] != "Etiqueta vip adicionada por Oliveira" || primeiro["autor"] != "Oliveira" {
		t.Errorf("tipo desconhecido devia vir com o texto e o autor: %v", primeiro)
	}
	if antigo, _ := hist[4].(map[string]any); antigo["texto"] != "Transferido para a equipe: pediu uma pessoa" || antigo["n"] == float64(0) {
		t.Errorf("registro antigo devia vir do banco, com o número dele: %v", antigo)
	}
	if r["historico_tem_mais"] != false {
		t.Errorf("historico_tem_mais = %v", r["historico_tem_mais"])
	}

	// Nenhuma mensagem da conversa entra na linha do tempo.
	bruto, _ := json.Marshal(r["historico"])
	if bytes.Contains(bruto, []byte("mensagem 1")) || bytes.Contains(bruto, []byte("quero um site")) {
		t.Errorf("a linha do tempo trouxe mensagens da conversa")
	}
}

// Um registro que ainda nao foi para o banco aparece na ficha, e nao aparece em dobro
// depois de gravado.
func TestFichaJuntaRegistrosDaMemoria(t *testing.T) {
	a, l, _ := appDaFicha(t)
	a.mu.Lock()
	l.evento("cancelou", "Horário cancelado pelo assistente: Reunião")
	a.mu.Unlock()

	conta := func() (int, string) {
		t.Helper()
		_, r := pedirFicha(t, a, "GET", "/api/contato?id=5527999990001%40s.whatsapp.net", nil)
		hist, _ := r["historico"].([]any)
		n := 0
		for _, x := range hist {
			if ev, _ := x.(map[string]any); ev["evento"] == "cancelou" {
				n++
			}
		}
		primeiro, _ := hist[0].(map[string]any)
		return n, fmt.Sprint(primeiro["evento"])
	}
	if n, primeiro := conta(); n != 1 || primeiro != "cancelou" {
		t.Errorf("antes de gravar: %d registros de cancelamento, o mais novo é %q", n, primeiro)
	}
	a.mu.Lock()
	a.salvarLeads()
	a.mu.Unlock()
	if n, primeiro := conta(); n != 1 || primeiro != "cancelou" {
		t.Errorf("depois de gravar: %d registros de cancelamento, o mais novo é %q", n, primeiro)
	}

	// Sem banco aberto, a linha do tempo sai so da memoria.
	semBanco := novoApp()
	bia := &Lead{ID: "5527999990003@s.whatsapp.net", Telefone: "5527999990003", NomeWhats: "Bia", CriadoEm: time.Now().Add(-time.Hour), UltimaMsg: time.Now()}
	bia.anexar(Msg{De: "cliente", Texto: "Bom dia", Hora: bia.CriadoEm})
	bia.evento("transbordo", "Transferido para a equipe: pediu uma pessoa")
	semBanco.leads[bia.ID] = bia
	codigo, r := pedirFicha(t, semBanco, "GET", "/api/contato?id=5527999990003%40s.whatsapp.net", nil)
	hist, _ := r["historico"].([]any)
	if codigo != 200 || len(hist) != 2 {
		t.Fatalf("sem banco: código %d, linha do tempo %v", codigo, r["historico"])
	}
	if novo, _ := hist[0].(map[string]any); novo["evento"] != "transbordo" {
		t.Errorf("sem banco: o registro da memória não veio: %v", hist)
	}
	if ags, tem := r["agendamentos"].([]any); !tem || len(ags) != 0 {
		t.Errorf("contato sem agenda devia trazer uma lista vazia, veio %v", r["agendamentos"])
	}
}

func TestLinhaDoTempoTemLimite(t *testing.T) {
	base := time.Date(2026, 1, 1, 9, 0, 0, 0, time.UTC)
	var doBanco []Msg
	for i := 9; i >= 0; i-- { // o banco entrega do mais novo para o mais antigo
		doBanco = append(doBanco, Msg{N: int64(i + 1), De: "sistema", Evento: "assumiu", Texto: fmt.Sprintf("registro %d", i), Hora: base.Add(time.Duration(i) * time.Minute)})
	}
	linha, cortou := linhaDoTempo(doBanco, nil, base.Add(-time.Hour), 4)
	if !cortou || len(linha) != 5 {
		t.Fatalf("esperava 4 registros mais o primeiro contato, com corte: %d registros, cortou=%v", len(linha), cortou)
	}
	if linha[0].Texto != "registro 9" || linha[3].Texto != "registro 6" || linha[4].Evento != "primeiro_contato" {
		t.Errorf("o corte devia ficar com os mais novos: %+v", linha)
	}
	if linha, cortou := linhaDoTempo(nil, nil, time.Time{}, 4); cortou || len(linha) != 0 {
		t.Errorf("contato sem data e sem registros devia dar uma linha vazia: %+v", linha)
	}
}

func TestFichaDeContatoQueNaoExiste(t *testing.T) {
	a, _, _ := appDaFicha(t)
	for _, caminho := range []string{"/api/contato?id=5527999990099%40s.whatsapp.net", "/api/contato?id=", "/api/contato"} {
		codigo, r := pedirFicha(t, a, "GET", caminho, nil)
		erro, _ := r["erro"].(string)
		if codigo != 404 || r["ok"] != false || !strings.Contains(erro, "Não achei esse contato") {
			t.Errorf("%s: código %d, resposta %v", caminho, codigo, r)
		}
		if r["contato"] != nil {
			t.Errorf("%s: a resposta de erro trouxe dados", caminho)
		}
	}
}

func TestAnotacoesDoContato(t *testing.T) {
	a, l, caminho := appDaFicha(t)
	const id = "5527999990001@s.whatsapp.net"
	const texto = "Ligar na sexta para confirmar o orçamento.\nPrefere falar à tarde."

	codigo, r := pedirFicha(t, a, "POST", "/api/contato/anotacoes", map[string]any{"id": id, "anotacoes": "  " + strings.ReplaceAll(texto, "\n", "\r\n") + "\x00 \n\n"})
	if codigo != 200 || r["ok"] != true || r["anotacoes"] != texto {
		t.Fatalf("salvando: código %d, resposta %v", codigo, r)
	}
	if l.Anotacoes != texto {
		t.Errorf("a memória ficou com %q", l.Anotacoes)
	}
	_, r = pedirFicha(t, a, "GET", "/api/contato?id=5527999990001%40s.whatsapp.net", nil)
	if c, _ := r["contato"].(map[string]any); c["anotacoes"] != texto {
		t.Errorf("a ficha não devolveu as anotações: %v", c["anotacoes"])
	}

	// Relido do banco, como se o programa tivesse sido religado.
	reler := func() *Lead {
		t.Helper()
		b2, err := abrirBanco(caminho)
		if err != nil {
			t.Fatal(err)
		}
		defer b2.fechar()
		leads, err := b2.lerLeads(maxHistorico)
		if err != nil {
			t.Fatal(err)
		}
		for _, x := range leads {
			if x.ID == id {
				return x
			}
		}
		t.Fatalf("o contato sumiu do banco")
		return nil
	}
	if relido := reler(); relido.Anotacoes != texto || relido.Nome != "Mariana Costa" || len(relido.Etiquetas) != 2 {
		t.Errorf("relido do banco: anotações %q, nome %q, etiquetas %v", relido.Anotacoes, relido.Nome, relido.Etiquetas)
	}

	// Texto grande demais e recusado e nao troca o que estava salvo.
	codigo, r = pedirFicha(t, a, "POST", "/api/contato/anotacoes", map[string]any{"id": id, "anotacoes": strings.Repeat("ç", maxAnotacoes+1)})
	if erro, _ := r["erro"].(string); codigo != 400 || !strings.Contains(erro, "4000") {
		t.Errorf("texto grande demais: código %d, resposta %v", codigo, r)
	}
	if l.Anotacoes != texto || reler().Anotacoes != texto {
		t.Errorf("o texto recusado trocou as anotações salvas")
	}
	// No limite exato passa (a conta e por letra, nao por byte).
	noLimite := strings.Repeat("ç", maxAnotacoes)
	codigo, _ = pedirFicha(t, a, "POST", "/api/contato/anotacoes", map[string]any{"id": id, "anotacoes": noLimite})
	if codigo != 200 || reler().Anotacoes != noLimite {
		t.Errorf("texto no limite: código %d", codigo)
	}

	// Apagar o texto tambem grava.
	codigo, r = pedirFicha(t, a, "POST", "/api/contato/anotacoes", map[string]any{"id": id, "anotacoes": "   "})
	if codigo != 200 || r["anotacoes"] != "" || reler().Anotacoes != "" {
		t.Errorf("apagando: código %d, resposta %v", codigo, r)
	}

	codigo, r = pedirFicha(t, a, "POST", "/api/contato/anotacoes", map[string]any{"id": "5527999990099@s.whatsapp.net", "anotacoes": "x"})
	if erro, _ := r["erro"].(string); codigo != 404 || !strings.Contains(erro, "Não achei esse contato") {
		t.Errorf("contato que não existe: código %d, resposta %v", codigo, r)
	}
	codigo, _ = pedirFicha(t, a, "GET", "/api/contato/anotacoes?id="+id, nil)
	if codigo != http.StatusMethodNotAllowed {
		t.Errorf("GET nas anotações: código %d, esperava 405", codigo)
	}
}

// As rotas da ficha estao ligadas no painel de verdade e nascem fechadas: sem login, nada feito.
func TestFichaExigeLogin(t *testing.T) {
	a, _, _ := appDaFicha(t)
	a.cookie = "atendente_teste"
	srv := httptest.NewServer(a.protegido(a.rotas()))
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/contato?id=5527999990001%40s.whatsapp.net")
	if err != nil {
		t.Fatal(err)
	}
	corpo := new(bytes.Buffer)
	_, _ = corpo.ReadFrom(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized || strings.Contains(corpo.String(), "Mariana") {
		t.Errorf("ficha sem login: código %d, corpo %s", resp.StatusCode, corpo.String())
	}

	resp, err = http.Post(srv.URL+"/api/contato/anotacoes", "application/json", strings.NewReader(`{"id":"5527999990001@s.whatsapp.net","anotacoes":"invasão"}`))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("anotações sem login: código %d", resp.StatusCode)
	}
	a.mu.Lock()
	gravou := a.leads["5527999990001@s.whatsapp.net"].Anotacoes
	a.mu.Unlock()
	if gravou != "" {
		t.Errorf("as anotações foram gravadas sem login: %q", gravou)
	}
	for _, rota := range []string{"/api/contato", "/api/contato/anotacoes"} {
		if nivelDaRota("POST", rota) != nivelLogado || nivelDaRota("GET", rota) != nivelLogado {
			t.Errorf("%s devia exigir login", rota)
		}
	}
}
