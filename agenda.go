package main

import (
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

// A agenda e simples de proposito: servicos com duracao, expediente por dia da
// semana e um limite de atendimentos ao mesmo tempo. O assistente so marca
// horarios que esta lista considera livres.

type Servico struct {
	Nome       string `json:"nome"`
	DuracaoMin int    `json:"duracao_min"`
	Preco      string `json:"preco"`
}

type Expediente struct {
	Dia         int    `json:"dia"` // 0 domingo ... 6 sabado
	Aberto      bool   `json:"aberto"`
	Inicio      string `json:"inicio"`
	Fim         string `json:"fim"`
	PausaInicio string `json:"pausa_inicio"`
	PausaFim    string `json:"pausa_fim"`
}

type AgendaConfig struct {
	Ativa           bool         `json:"ativa"`
	Servicos        []Servico    `json:"servicos"`
	Dias            []Expediente `json:"dias"`
	IntervaloMin    int          `json:"intervalo_min"`    // de quanto em quanto tempo abre um horario
	Vagas           int          `json:"vagas"`            // atendimentos ao mesmo tempo
	AntecedenciaMin int          `json:"antecedencia_min"` // minimo de minutos antes do horario
	DiasAFrente     int          `json:"dias_a_frente"`
	AvisarHumano    bool         `json:"avisar_humano"`
}

type Agendamento struct {
	ID         string    `json:"id"`
	LeadID     string    `json:"lead_id"`
	Nome       string    `json:"nome"`
	Telefone   string    `json:"telefone"`
	Servico    string    `json:"servico"`
	Inicio     time.Time `json:"inicio"`
	DuracaoMin int       `json:"duracao_min"`
	Origem     string    `json:"origem"` // "assistente" ou "painel"
	Status     string    `json:"status"` // "marcado" ou "cancelado"
	Obs        string    `json:"obs"`
	CriadoEm   time.Time `json:"criado_em"`
}

func (ag *Agendamento) fim() time.Time {
	return ag.Inicio.Add(time.Duration(ag.DuracaoMin) * time.Minute)
}

// minutos le "09:30" e devolve 570.
func minutos(s string) (int, bool) {
	partes := strings.Split(strings.TrimSpace(s), ":")
	if len(partes) != 2 {
		return 0, false
	}
	h, err1 := strconv.Atoi(partes[0])
	m, err2 := strconv.Atoi(partes[1])
	if err1 != nil || err2 != nil || h < 0 || h > 24 || m < 0 || m > 59 || h*60+m > 24*60 {
		return 0, false
	}
	return h*60 + m, true
}

func hhmm(m int) string {
	return fmt.Sprintf("%02d:%02d", m/60, m%60)
}

func (c *AgendaConfig) normalizar() {
	novo := len(c.Dias) != 7
	if novo {
		c.Dias = make([]Expediente, 7)
		for d := 0; d < 7; d++ {
			c.Dias[d] = Expediente{Dia: d, Aberto: d >= 1 && d <= 5, Inicio: "09:00", Fim: "18:00"}
		}
		c.Dias[6] = Expediente{Dia: 6, Aberto: true, Inicio: "09:00", Fim: "13:00"}
		c.AvisarHumano = true
		if c.AntecedenciaMin == 0 {
			c.AntecedenciaMin = 60
		}
	}
	for d := range c.Dias {
		e := &c.Dias[d]
		e.Dia = d
		ini, ok1 := minutos(e.Inicio)
		fim, ok2 := minutos(e.Fim)
		if !ok1 || !ok2 || fim <= ini {
			e.Inicio, e.Fim = "09:00", "18:00"
			ini, fim = 9*60, 18*60
		}
		e.Inicio, e.Fim = hhmm(ini), hhmm(fim)
		pi, ok3 := minutos(e.PausaInicio)
		pf, ok4 := minutos(e.PausaFim)
		if !ok3 || !ok4 || pf <= pi || pi < ini || pf > fim {
			e.PausaInicio, e.PausaFim = "", ""
		} else {
			e.PausaInicio, e.PausaFim = hhmm(pi), hhmm(pf)
		}
	}
	if c.IntervaloMin <= 0 {
		c.IntervaloMin = 30
	}
	c.IntervaloMin = limitar(c.IntervaloMin, 5, 240)
	if c.Vagas <= 0 {
		c.Vagas = 1
	}
	c.Vagas = limitar(c.Vagas, 1, 50)
	c.AntecedenciaMin = limitar(c.AntecedenciaMin, 0, 7*24*60)
	if c.DiasAFrente <= 0 {
		c.DiasAFrente = 7
	}
	c.DiasAFrente = limitar(c.DiasAFrente, 1, 60)

	servicos := make([]Servico, 0, len(c.Servicos))
	for _, s := range c.Servicos {
		nome := cortar(strings.TrimSpace(s.Nome), 120)
		if nome == "" || len(servicos) >= 50 {
			continue
		}
		dur := s.DuracaoMin
		if dur <= 0 {
			dur = c.IntervaloMin
		}
		servicos = append(servicos, Servico{Nome: nome, DuracaoMin: limitar(dur, 5, 600), Preco: cortar(strings.TrimSpace(s.Preco), 80)})
	}
	c.Servicos = servicos
}

func chaveTexto(s string) string {
	return strings.ToLower(semAcento(umaLinha(s)))
}

// servicoDe acha o servico pelo nome que o cliente (ou a IA) usou. Se nao achar,
// usa o texto recebido com a duracao padrao: nome diferente nao impede de marcar.
func (c AgendaConfig) servicoDe(texto string) (string, int) {
	t := chaveTexto(texto)
	if t != "" {
		for _, s := range c.Servicos {
			if chaveTexto(s.Nome) == t {
				return s.Nome, s.DuracaoMin
			}
		}
		for _, s := range c.Servicos {
			n := chaveTexto(s.Nome)
			if strings.Contains(n, t) || strings.Contains(t, n) {
				return s.Nome, s.DuracaoMin
			}
		}
	}
	if len(c.Servicos) == 1 {
		return c.Servicos[0].Nome, c.Servicos[0].DuracaoMin
	}
	return cortar(strings.TrimSpace(texto), 120), c.IntervaloMin
}

// menorDuracao e a duracao usada para listar horarios livres quando ainda nao se sabe o servico.
func (c AgendaConfig) menorDuracao() int {
	menor := 0
	for _, s := range c.Servicos {
		if menor == 0 || s.DuracaoMin < menor {
			menor = s.DuracaoMin
		}
	}
	if menor == 0 {
		return c.IntervaloMin
	}
	return menor
}

// ocupados conta os agendamentos ativos que cruzam o intervalo [inicio, fim).
func ocupados(ags []*Agendamento, inicio, fim time.Time) int {
	n := 0
	for _, ag := range ags {
		if ag.Status != "marcado" {
			continue
		}
		if ag.Inicio.Before(fim) && ag.fim().After(inicio) {
			n++
		}
	}
	return n
}

// cabeNoExpediente diz se [inicio, inicio+dur) fica dentro do horario do dia e fora da pausa.
func (c AgendaConfig) cabeNoExpediente(inicio time.Time, dur int) bool {
	e := c.Dias[int(inicio.Weekday())]
	if !e.Aberto {
		return false
	}
	ini, _ := minutos(e.Inicio)
	fim, _ := minutos(e.Fim)
	m := inicio.Hour()*60 + inicio.Minute()
	if m < ini || m+dur > fim {
		return false
	}
	if pi, ok := minutos(e.PausaInicio); ok {
		if pf, ok := minutos(e.PausaFim); ok && m < pf && m+dur > pi {
			return false
		}
	}
	return true
}

var errHorarioCheio = errors.New("esse horário já está ocupado")

// conferir diz se da para marcar. Pelo painel a pessoa pode marcar fora do
// expediente e em cima da hora; o assistente nao pode.
func (c AgendaConfig) conferir(ags []*Agendamento, inicio time.Time, dur int, agora time.Time, doPainel bool) error {
	if !inicio.After(agora) {
		return errors.New("esse horário já passou")
	}
	if !doPainel {
		if inicio.Before(agora.Add(time.Duration(c.AntecedenciaMin) * time.Minute)) {
			return errors.New("esse horário está muito em cima da hora")
		}
		limite := time.Date(agora.Year(), agora.Month(), agora.Day(), 0, 0, 0, 0, agora.Location()).AddDate(0, 0, c.DiasAFrente+1)
		if !inicio.Before(limite) {
			return errors.New("esse dia está longe demais para marcar agora")
		}
		if !c.cabeNoExpediente(inicio, dur) {
			return errors.New("esse horário fica fora do atendimento")
		}
	}
	if ocupados(ags, inicio, inicio.Add(time.Duration(dur)*time.Minute)) >= c.Vagas {
		return errHorarioCheio
	}
	return nil
}

// horariosLivres devolve os horarios de um dia em que cabe um servico de dur minutos.
func (c AgendaConfig) horariosLivres(ags []*Agendamento, dia time.Time, dur int, agora time.Time) []string {
	e := c.Dias[int(dia.Weekday())]
	if !e.Aberto {
		return nil
	}
	ini, _ := minutos(e.Inicio)
	fim, _ := minutos(e.Fim)
	var livres []string
	for m := ini; m+dur <= fim; m += c.IntervaloMin {
		inicio := time.Date(dia.Year(), dia.Month(), dia.Day(), m/60, m%60, 0, 0, dia.Location())
		if c.conferir(ags, inicio, dur, agora, false) == nil {
			livres = append(livres, hhmm(m))
		}
	}
	return livres
}

type diaLivre struct {
	Data   string   `json:"data"`
	Rotulo string   `json:"rotulo"`
	Horas  []string `json:"horas"`
}

// livresProximosDias lista os horarios livres de hoje ate o limite configurado.
func (c AgendaConfig) livresProximosDias(ags []*Agendamento, dur int, agora time.Time) []diaLivre {
	var dias []diaLivre
	for d := 0; d <= c.DiasAFrente; d++ {
		dia := agora.AddDate(0, 0, d)
		horas := c.horariosLivres(ags, dia, dur, agora)
		if len(horas) == 0 {
			continue
		}
		dias = append(dias, diaLivre{Data: dia.Format("2006-01-02"), Rotulo: diaCurto(dia), Horas: horas})
	}
	return dias
}

// sugestoes devolve ate n horarios livres, comecando pelo dia pedido.
func (c AgendaConfig) sugestoes(ags []*Agendamento, perto time.Time, dur, n int, agora time.Time) []string {
	var lista []string
	dias := c.livresProximosDias(ags, dur, agora)
	chave := perto.Format("2006-01-02")
	// Primeiro o dia pedido, depois os outros na ordem.
	sort.SliceStable(dias, func(i, j int) bool { return dias[i].Data == chave && dias[j].Data != chave })
	for _, d := range dias {
		for _, h := range d.Horas {
			if len(lista) >= n {
				return lista
			}
			lista = append(lista, d.Rotulo+" às "+h)
		}
	}
	return lista
}

// interpretarDataHora entende "2026-11-12" ou "12/11" e "14:00", "14h30" ou "14".
func interpretarDataHora(data, hora string, agora time.Time) (time.Time, error) {
	data = strings.TrimSpace(data)
	hora = strings.ToLower(strings.TrimSpace(hora))
	var ano, mes, dia int
	if t, err := time.ParseInLocation("2006-01-02", data, agora.Location()); err == nil {
		ano, mes, dia = t.Year(), int(t.Month()), t.Day()
	} else if t, err := time.ParseInLocation("02/01/2006", data, agora.Location()); err == nil {
		ano, mes, dia = t.Year(), int(t.Month()), t.Day()
	} else if t, err := time.ParseInLocation("02/01", data, agora.Location()); err == nil {
		ano, mes, dia = agora.Year(), int(t.Month()), t.Day()
		if time.Date(ano, time.Month(mes), dia, 23, 59, 0, 0, agora.Location()).Before(agora) {
			ano++
		}
	} else {
		return time.Time{}, errors.New("não entendi o dia")
	}

	hora = strings.NewReplacer("hs", ":", "h", ":", " ", "").Replace(hora)
	hora = strings.TrimSuffix(hora, ":")
	if !strings.Contains(hora, ":") {
		hora += ":00"
	}
	m, ok := minutos(hora)
	if !ok || m >= 24*60 {
		return time.Time{}, errors.New("não entendi a hora")
	}
	return time.Date(ano, time.Month(mes), dia, m/60, m%60, 0, 0, agora.Location()), nil
}

// marcar cria o agendamento se o horario estiver livre. Chame com a.mu travado.
func (a *App) marcar(nome, telefone, leadID, servico string, inicio time.Time, origem, obs string, agora time.Time) (*Agendamento, error) {
	cfg := a.cfg.Agenda
	nomeServico, dur := cfg.servicoDe(servico)
	if err := cfg.conferir(a.agenda, inicio, dur, agora, origem == "painel"); err != nil {
		return nil, err
	}
	ag := &Agendamento{
		ID:         novoID("a"),
		LeadID:     leadID,
		Nome:       cortar(strings.TrimSpace(nome), 120),
		Telefone:   soDigitos(telefone),
		Servico:    nomeServico,
		Inicio:     inicio,
		DuracaoMin: dur,
		Origem:     origem,
		Status:     "marcado",
		Obs:        cortar(strings.TrimSpace(obs), 500),
		CriadoEm:   agora,
	}
	a.agenda = append(a.agenda, ag)
	a.salvarAgenda()
	return ag, nil
}

// futurosDoLead devolve os agendamentos ativos e futuros de uma conversa. Chame com a.mu travado.
func (a *App) futurosDoLead(leadID string, agora time.Time) []*Agendamento {
	var lista []*Agendamento
	if leadID == "" {
		return lista
	}
	for _, ag := range a.agenda {
		if ag.LeadID == leadID && ag.Status == "marcado" && ag.Inicio.After(agora) {
			lista = append(lista, ag)
		}
	}
	sort.Slice(lista, func(i, j int) bool { return lista[i].Inicio.Before(lista[j].Inicio) })
	return lista
}

// resultadoAgenda conta o que o pedido de agenda da IA causou.
type resultadoAgenda struct {
	Resposta   string       // se nao for vazio, troca a resposta que iria para o cliente
	Transbordo bool         // nao achou saida: chama uma pessoa
	Marcado    *Agendamento // avisar o numero humano
	Cancelado  *Agendamento
}

// aplicarAgendamento executa o pedido de agenda que veio do cerebro. Chame com a.mu travado.
func (a *App) aplicarAgendamento(l *Lead, pedido AgendamentoIA, nomeCliente string, agora time.Time) resultadoAgenda {
	var r resultadoAgenda
	cfg := a.cfg.Agenda
	acao := strings.ToLower(strings.TrimSpace(pedido.Acao))
	if !cfg.Ativa || (acao != "marcar" && acao != "cancelar") {
		return r
	}
	futuros := a.futurosDoLead(l.ID, agora)
	inicio, errData := interpretarDataHora(pedido.Data, pedido.Hora, agora)

	if acao == "cancelar" {
		var alvo *Agendamento
		for _, ag := range futuros {
			if errData == nil && ag.Inicio.Equal(inicio) {
				alvo = ag
			}
		}
		if alvo == nil && len(futuros) > 0 {
			alvo = futuros[0]
		}
		if alvo != nil {
			alvo.Status = "cancelado"
			a.salvarAgenda()
			r.Cancelado = alvo
		}
		return r
	}

	if errData != nil {
		r.Resposta = "Para eu marcar certinho, me diga de novo o dia e a hora que você prefere?"
		return r
	}
	for _, ag := range futuros {
		if ag.Inicio.Equal(inicio) {
			return r // ja estava marcado: a IA so repetiu o pedido
		}
	}
	ag, err := a.marcar(ouPadrao(nomeCliente, l.exibir()), l.Telefone, l.ID, pedido.Servico, inicio, "assistente", "", agora)
	if err == nil {
		r.Marcado = ag
		return r
	}
	_, dur := cfg.servicoDe(pedido.Servico)
	opcoes := cfg.sugestoes(a.agenda, inicio, dur, 4, agora)
	if len(opcoes) == 0 {
		r.Resposta = "Não encontrei um horário livre nos próximos dias. Estou transferindo o seu atendimento para a equipe responsável pela agenda, que continua com você por aqui."
		r.Transbordo = true
		return r
	}
	r.Resposta = "Esse horário não está disponível. Tenho estes: " + strings.Join(opcoes, ", ") + ". Qual você prefere?"
	return r
}

// blocoAgenda monta a parte das instrucoes que fala da agenda.
func (a *App) blocoAgenda(cfg Config, leadID string, agora time.Time) string {
	ag := cfg.Agenda
	if !ag.Ativa {
		return ""
	}
	a.mu.Lock()
	copia := append([]*Agendamento(nil), a.agenda...)
	futuros := a.futurosDoLead(leadID, agora)
	a.mu.Unlock()

	var b strings.Builder
	b.WriteString("AGENDA (você pode marcar horários para o cliente)\n")
	if len(ag.Servicos) > 0 {
		b.WriteString("Serviços que podem ser agendados:\n")
		for _, s := range ag.Servicos {
			linha := fmt.Sprintf("- %s (%d min)", s.Nome, s.DuracaoMin)
			if s.Preco != "" {
				linha += ", " + s.Preco
			}
			b.WriteString(linha + "\n")
		}
	}
	dias := ag.livresProximosDias(copia, ag.menorDuracao(), agora)
	if len(dias) == 0 {
		b.WriteString("Não há horário livre nos próximos dias. Se o cliente quiser marcar, chame uma pessoa.\n")
	} else {
		b.WriteString("Horários livres (use SOMENTE horários desta lista):\n")
		for _, d := range dias {
			horas := d.Horas
			if len(horas) > 28 {
				horas = horas[:28]
			}
			b.WriteString("- " + d.Rotulo + " (" + d.Data + "): " + strings.Join(horas, ", ") + "\n")
		}
	}
	if len(futuros) > 0 {
		b.WriteString("Este cliente já tem marcado:\n")
		for _, f := range futuros {
			b.WriteString("- " + f.Servico + " em " + dataPorExtenso(f.Inicio) + "\n")
		}
	}
	b.WriteString(`Como marcar: descubra o serviço e o dia que a pessoa prefere, ofereça de 2 a 4 horários livres desse dia e espere a pessoa escolher. Só na mensagem em que ela confirmar o horário, preencha agendamento com acao "marcar", o serviço, a data no formato AAAA-MM-DD e a hora no formato HH:MM, e diga que ficou marcado. Nas outras mensagens use acao "nenhuma". Se a pessoa pedir para desmarcar, use acao "cancelar" com a data e a hora do agendamento. Para trocar de horário, primeiro cancele e depois marque o novo. Nunca confirme um horário que não esteja na lista.
Querer marcar um horário NÃO é motivo para chamar uma pessoa, mesmo que o lead seja quente: marque você mesmo. Chame uma pessoa só pelos outros motivos das regras de transbordo.`)
	return b.String()
}

// avisoAgendamento e o texto que vai para o numero humano.
func avisoAgendamento(ag *Agendamento, cancelado bool) string {
	titulo := "AGENDAMENTO"
	if cancelado {
		titulo = "AGENDAMENTO CANCELADO"
	}
	var b strings.Builder
	b.WriteString(titulo + ": " + ouPadrao(ag.Nome, "sem nome") + "\n")
	b.WriteString("Serviço: " + ouPadrao(ag.Servico, "não informado") + "\n")
	b.WriteString("Quando: " + dataPorExtenso(ag.Inicio) + "\n")
	b.WriteString("Telefone: " + telefoneBonito(ag.Telefone) + "\n")
	b.WriteString("Feito pelo assistente. Veja a agenda no painel.")
	return b.String()
}
