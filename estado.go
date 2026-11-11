package main

import (
	crand "crypto/rand"
	"encoding/hex"
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/store/sqlstore"
)

const (
	versao = "2.1.0"

	statusQualificando = "qualificando"
	statusQualificado  = "qualificado"
	statusHumano       = "aguardando humano" // na fila, esperando uma pessoa
	statusAtendimento  = "em atendimento"    // uma pessoa assumiu

	maxHistorico   = 80   // mensagens guardadas por lead
	maxTextoMsg    = 2000 // corta mensagens gigantes
	janelaLimite   = 10 * time.Minute
	idadeMaximaMsg = 10 * time.Minute // mensagens mais velhas so sao registradas

	modeloPadrao   = "claude-haiku-5-5"
	modeloAjudante = "claude-sonnet-5-5" // escreve rascunhos e organiza materiais
	tomPadrao      = "simpático e profissional"
)

// Tudo o que e da pessoa (dados, conversas, sessao do WhatsApp) fica em dados/.
// Essa pasta nunca vai para o repositorio.
const pastaDados = "dados"

var (
	arqConfig         = filepath.Join(pastaDados, "config.json")
	arqLeads          = filepath.Join(pastaDados, "leads.json")
	arqAgenda         = filepath.Join(pastaDados, "agenda.json")
	arqSessao         = filepath.Join(pastaDados, "sessao.db")
	pastaTextos       = filepath.Join(pastaDados, "textos")
	pastaConhecimento = filepath.Join(pastaDados, "conhecimento")
	arqPersonalidade  = filepath.Join(pastaTextos, "personalidade.md")
	arqCerebro        = filepath.Join(pastaTextos, "cerebro.md")
	arqRegras         = filepath.Join(pastaTextos, "regras-transbordo.md")
	arqAprendizados   = filepath.Join(pastaTextos, "aprendizados.md")
)

// Variavel e uma informacao extra criada pela pessoa. O nome vira {{NOME}} nos textos.
type Variavel struct {
	Nome  string `json:"nome"`
	Valor string `json:"valor"`
}

// Config guarda o que o dono preencheu no painel.
type Config struct {
	Negocio      string     `json:"negocio"`
	Segmento     string     `json:"segmento"`
	Cidade       string     `json:"cidade"`
	Endereco     string     `json:"endereco"`
	Telefone     string     `json:"telefone"`
	Site         string     `json:"site"`
	Instagram    string     `json:"instagram"`
	Produtos     string     `json:"produtos"`
	Horario      string     `json:"horario"`
	Pagamento    string     `json:"pagamento"`
	Diferenciais string     `json:"diferenciais"`
	Promocoes    string     `json:"promocoes"`
	Politicas    string     `json:"politicas"`
	Extras       []Variavel `json:"extras"`

	Assistente string `json:"assistente"`
	Tom        string `json:"tom"`

	NumeroHumano   string       `json:"numero_humano"`
	NumerosHumanos listaNumeros `json:"numeros_humanos"`
	NuncaResponder listaNumeros `json:"nunca_responder"`

	Modelo     string `json:"modelo"`
	AtrasoMin  int    `json:"atraso_min"` // segundos antes de responder
	AtrasoMax  int    `json:"atraso_max"`
	LimiteMsgs int    `json:"limite_msgs"` // mensagens por contato a cada 10 minutos

	Agenda AgendaConfig `json:"agenda"`

	Salvo  bool `json:"salvo"`
	Ligado bool `json:"ligado"`
}

// listaNumeros aceita uma lista ["55...", "55..."] ou um texto com um numero por
// linha (ou separados por virgula), que e como a pessoa digita.
type listaNumeros []string

func (l *listaNumeros) UnmarshalJSON(b []byte) error {
	var lista []string
	if err := json.Unmarshal(b, &lista); err == nil {
		*l = lista
		return nil
	}
	var texto string
	if err := json.Unmarshal(b, &texto); err != nil {
		return err
	}
	*l = strings.FieldsFunc(texto, func(r rune) bool { return r == ',' || r == ';' || r == '\n' })
	return nil
}

type modeloOpcao struct {
	ID   string `json:"id"`
	Nome string `json:"nome"`
	Dica string `json:"dica"`
}

var modelosDisponiveis = []modeloOpcao{
	{"claude-haiku-5-5", "Haiku 5.5", "O mais rápido. Bom para o dia a dia."},
	{"claude-sonnet-5-5", "Sonnet 5.5", "Mais caprichado, um pouco mais lento."},
	{"claude-opus-5-5", "Opus 5.5", "O mais inteligente e o mais lento."},
}

var tonsDisponiveis = []string{
	"simpático e profissional",
	"descontraído e próximo, como um amigo",
	"formal e elegante",
	"direto e objetivo",
	"animado e cheio de energia",
	"calmo e acolhedor",
}

func limitar(v, min, max int) int {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}

// nomeVariavel deixa o nome de uma variavel extra so com letras maiusculas, numeros e _.
func nomeVariavel(s string) string {
	s = strings.ToUpper(semAcento(strings.TrimSpace(s)))
	var b strings.Builder
	for _, r := range s {
		switch {
		case (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9'):
			b.WriteRune(r)
		case r == ' ' || r == '_' || r == '-':
			b.WriteRune('_')
		}
	}
	return cortar(strings.Trim(b.String(), "_"), 40)
}

// normalizar aplica limites e valores padrao. Vale para o que vem do disco e do painel.
func (c *Config) normalizar() {
	limpo := func(s string, max int) string { return cortar(strings.TrimSpace(s), max) }
	c.Negocio = limpo(c.Negocio, 200)
	c.Segmento = limpo(c.Segmento, 200)
	c.Cidade = limpo(c.Cidade, 200)
	c.Endereco = limpo(c.Endereco, 400)
	c.Telefone = limpo(c.Telefone, 80)
	c.Site = limpo(c.Site, 200)
	c.Instagram = limpo(c.Instagram, 200)
	c.Produtos = limpo(c.Produtos, 20000)
	c.Horario = limpo(c.Horario, 1000)
	c.Pagamento = limpo(c.Pagamento, 1000)
	c.Diferenciais = limpo(c.Diferenciais, 3000)
	c.Promocoes = limpo(c.Promocoes, 3000)
	c.Politicas = limpo(c.Politicas, 3000)

	extras := make([]Variavel, 0, len(c.Extras))
	vistos := map[string]bool{}
	for _, v := range c.Extras {
		nome := nomeVariavel(v.Nome)
		if nome == "" || vistos[nome] || variavelReservada(nome) || len(extras) >= 40 {
			continue
		}
		vistos[nome] = true
		extras = append(extras, Variavel{Nome: nome, Valor: limpo(v.Valor, 3000)})
	}
	c.Extras = extras

	c.Assistente = cortar(ouPadrao(c.Assistente, "Ana"), 80)
	c.Tom = cortar(ouPadrao(c.Tom, tomPadrao), 200)

	// Numeros que recebem os avisos. O campo antigo, de um numero so, ainda e aceito.
	fonte := []string(c.NumerosHumanos)
	if len(fonte) == 0 && strings.TrimSpace(c.NumeroHumano) != "" {
		fonte = strings.FieldsFunc(c.NumeroHumano, func(r rune) bool { return r == ',' || r == ';' || r == '\n' })
	}
	humanos := make([]string, 0, len(fonte))
	for _, n := range fonte {
		d := soDigitos(n)
		if len(d) == 10 || len(d) == 11 { // numero brasileiro digitado sem o 55
			d = "55" + d
		}
		repetido := false
		for _, h := range humanos {
			repetido = repetido || mesmoNumero(h, d)
		}
		if len(d) >= 10 && len(d) <= 15 && !repetido && len(humanos) < 10 {
			humanos = append(humanos, d)
		}
	}
	c.NumerosHumanos = humanos
	c.NumeroHumano = ""
	if len(humanos) > 0 {
		c.NumeroHumano = humanos[0]
	}
	nunca := make([]string, 0, len(c.NuncaResponder))
	for _, n := range c.NuncaResponder {
		if d := soDigitos(n); len(d) >= 8 && len(d) <= 15 && len(nunca) < 500 {
			nunca = append(nunca, d)
		}
	}
	c.NuncaResponder = nunca

	valido := false
	for _, m := range modelosDisponiveis {
		if m.ID == c.Modelo {
			valido = true
		}
	}
	if !valido {
		c.Modelo = modeloPadrao
	}
	if c.AtrasoMax <= 0 {
		c.AtrasoMin, c.AtrasoMax = 3, 8
	}
	c.AtrasoMin = limitar(c.AtrasoMin, 0, 60)
	c.AtrasoMax = limitar(c.AtrasoMax, 1, 60)
	if c.AtrasoMin > c.AtrasoMax {
		c.AtrasoMin = c.AtrasoMax
	}
	if c.LimiteMsgs <= 0 {
		c.LimiteMsgs = 6
	}
	c.LimiteMsgs = limitar(c.LimiteMsgs, 1, 60)
	c.Agenda.normalizar()
}

// Msg e uma mensagem da conversa.
type Msg struct {
	De       string    `json:"de"` // "cliente", "atendente" (a IA), "humano" (a equipe) ou "sistema"
	Texto    string    `json:"texto"`
	Hora     time.Time `json:"hora"`
	Pendente bool      `json:"pendente,omitempty"`

	ID      string `json:"id,omitempty"`    // id da mensagem no WhatsApp
	Tipo    string `json:"tipo,omitempty"`  // vazio (texto), imagem, audio, video, documento ou evento
	Midia   string `json:"midia,omitempty"` // nome do arquivo em dados/midia
	Nome    string `json:"nome,omitempty"`  // nome original do arquivo
	Mime    string `json:"mime,omitempty"`
	Tamanho int    `json:"tamanho,omitempty"`
	Autor   string `json:"autor,omitempty"`  // quem da equipe escreveu (so aparece no painel)
	Evento  string `json:"evento,omitempty"` // transbordo, assumiu, devolveu, agendou, cancelou
}

// textoPlano devolve o texto da mensagem; em midia sem legenda, diz o que ela e.
func (m Msg) textoPlano() string {
	if strings.TrimSpace(m.Texto) != "" {
		return m.Texto
	}
	switch m.Tipo {
	case "imagem":
		return "[imagem]"
	case "audio":
		return "[áudio]"
	case "video":
		return "[vídeo]"
	case "documento":
		return strings.TrimSpace("[arquivo] " + m.Nome)
	}
	return m.Texto
}

// Lead e uma pessoa que escreveu para o negocio.
type Lead struct {
	ID            string    `json:"id"`
	Telefone      string    `json:"telefone"`
	Nome          string    `json:"nome"`
	NomeWhats     string    `json:"nome_whatsapp"`
	Necessidade   string    `json:"necessidade"`
	Orcamento     string    `json:"orcamento"`
	Decisor       string    `json:"decisor"`
	Prazo         string    `json:"prazo"`
	Pontuacao     int       `json:"pontuacao"`
	Classificacao string    `json:"classificacao"`
	Etapa         string    `json:"etapa"`
	Resumo        string    `json:"resumo"`
	Status        string    `json:"status"`
	Motivo        string    `json:"motivo"`
	Retomado      bool      `json:"retomado"`
	AvisouMidia   bool      `json:"avisou_midia"`
	CriadoEm      time.Time `json:"criado_em"`
	UltimaMsg     time.Time `json:"ultima_msg"`
	Etiquetas     []string  `json:"etiquetas"`
	Atendente     string    `json:"atendente"` // quem da equipe esta com a conversa
	FotoID        string    `json:"foto_id"`   // vazio = sem foto de perfil
	FotoEm        time.Time `json:"foto_em"`   // ultima vez que a foto foi conferida
	Historico     []Msg     `json:"historico"`
}

// exibir devolve o melhor nome para mostrar na tela.
func (l *Lead) exibir() string {
	return ouPadrao(l.Nome, ouPadrao(l.NomeWhats, telefoneBonito(l.Telefone)))
}

// contato guarda o controle de ritmo de cada conversa (so em memoria).
type contato struct {
	janela       []time.Time
	avisouLimite bool
	trabalhando  bool
}

// App junta tudo: dados, WhatsApp, cerebro, agenda, base de conhecimento e painel.
type App struct {
	mu       sync.Mutex
	cfg      Config
	leads    map[string]*Lead
	contatos map[string]*contato
	agenda   []*Agendamento

	cli       *whatsmeow.Client
	container *sqlstore.Container
	trocar    chan struct{} // pede para refazer a conexao (aparelho removido)
	status    string        // iniciando, aguardando, conectando, conectado, desconectado
	mensagem  string
	erro      string
	qr        string
	nomeConta string
	numConta  string

	cerebro  *Cerebro
	base     *Base
	vagas    chan struct{} // no maximo 3 chamadas ao cerebro ao mesmo tempo
	ajuda    chan struct{} // no maximo 2 pedidos de ajuda (rascunhos, resumos) ao mesmo tempo
	baixando chan struct{} // no maximo 2 midias sendo baixadas ao mesmo tempo
	digitam  int           // quantas conversas estao com "digitando"
	porta    int
	iniciado time.Time
}

func novoApp() *App {
	return &App{
		leads:    map[string]*Lead{},
		contatos: map[string]*contato{},
		trocar:   make(chan struct{}, 1),
		status:   "iniciando",
		mensagem: "Iniciando...",
		cerebro:  &Cerebro{},
		base:     novaBase(),
		vagas:    make(chan struct{}, 3),
		ajuda:    make(chan struct{}, 2),
		baixando: make(chan struct{}, 2),
		iniciado: time.Now(),
	}
}

// migrarDadosAntigos leva os arquivos da primeira versao (soltos na pasta) para dados/.
func migrarDadosAntigos() {
	mover := func(de, para string) {
		if !existeArquivo(de) || existeArquivo(para) {
			return
		}
		if err := os.Rename(de, para); err != nil {
			log.Printf("AVISO: não consegui mover %s para %s: %v", de, para, err)
			return
		}
		log.Printf("MIGRAÇÃO: %s agora fica em %s", de, para)
	}
	mover("config.json", arqConfig)
	mover("leads.json", arqLeads)
	if existeArquivo("sessao.db") && !existeArquivo(arqSessao) {
		for _, fim := range []string{"-wal", "-shm", ""} {
			mover("sessao.db"+fim, arqSessao+fim)
		}
	}
	// Os textos antigos eram gerados com os dados ja escritos dentro. Os novos usam
	// variaveis, entao os antigos ficam guardados so como copia.
	for _, nome := range []string{"personalidade.md", "cerebro.md", "regras-transbordo.md"} {
		mover(nome, filepath.Join(pastaTextos, nome+".anterior"))
		_ = os.Remove(nome + ".anterior")
	}
}

// carregar prepara a pasta dados/ e le o que ja existir nela.
func (a *App) carregar() {
	for _, p := range []string{pastaDados, pastaTextos, pastaConhecimento, pastaMidia, pastaFotos} {
		if err := os.MkdirAll(p, 0o700); err != nil {
			log.Printf("ERRO: não consegui criar a pasta %s: %v", p, err)
		}
	}
	migrarDadosAntigos()

	a.mu.Lock()
	defer a.mu.Unlock()
	if b, err := os.ReadFile(arqConfig); err == nil {
		if err := json.Unmarshal(b, &a.cfg); err != nil {
			log.Printf("AVISO: config.json com problema, ignorando: %v", err)
			a.cfg = Config{}
		}
	}
	a.cfg.normalizar()

	if b, err := os.ReadFile(arqLeads); err == nil {
		var lista []*Lead
		if err := json.Unmarshal(b, &lista); err != nil {
			log.Printf("AVISO: leads.json com problema. Guardei uma cópia em leads.json.quebrado: %v", err)
			_ = os.WriteFile(arqLeads+".quebrado", b, 0o600)
		}
		for _, l := range lista {
			if l == nil || l.ID == "" {
				continue
			}
			// Nada fica "pendente" de uma execucao para outra: nao respondemos mensagem velha.
			for i := range l.Historico {
				l.Historico[i].Pendente = false
			}
			if l.Status == "" {
				l.Status = statusQualificando
			}
			if l.Classificacao == "" {
				l.Classificacao = "indefinido"
			}
			a.leads[l.ID] = l
		}
	}

	if b, err := os.ReadFile(arqAgenda); err == nil {
		var lista []*Agendamento
		if err := json.Unmarshal(b, &lista); err != nil {
			log.Printf("AVISO: agenda.json com problema. Guardei uma cópia em agenda.json.quebrado: %v", err)
			_ = os.WriteFile(arqAgenda+".quebrado", b, 0o600)
		}
		for _, ag := range lista {
			if ag != nil && ag.ID != "" {
				a.agenda = append(a.agenda, ag)
			}
		}
	}
	a.base.carregar()
}

// gravarSeguro escreve em um arquivo temporario e troca de uma vez so.
func gravarSeguro(caminho string, dados []byte, modo os.FileMode) error {
	dir := filepath.Dir(caminho)
	tmp, err := os.CreateTemp(dir, filepath.Base(caminho)+".tmp-*")
	if err != nil {
		return err
	}
	nome := tmp.Name()
	limpar := func() { _ = os.Remove(nome) }
	if _, err := tmp.Write(dados); err != nil {
		tmp.Close()
		limpar()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		limpar()
		return err
	}
	if err := tmp.Close(); err != nil {
		limpar()
		return err
	}
	_ = os.Chmod(nome, modo)
	if err := os.Rename(nome, caminho); err != nil {
		limpar()
		return err
	}
	return nil
}

func gravarJSON(caminho string, v any) {
	b, err := json.MarshalIndent(v, "", "  ")
	if err == nil {
		err = gravarSeguro(caminho, b, 0o600)
	}
	if err != nil {
		log.Printf("ERRO ao salvar %s: %v", caminho, err)
	}
}

// salvarConfig, salvarLeads e salvarAgenda: chame com a.mu travado.
func (a *App) salvarConfig() { gravarJSON(arqConfig, a.cfg) }
func (a *App) salvarLeads()  { gravarJSON(arqLeads, a.leadsOrdenados()) }
func (a *App) salvarAgenda() { gravarJSON(arqAgenda, a.agenda) }

// leadsOrdenados devolve os leads, o mais recente primeiro. Chame com a.mu travado.
func (a *App) leadsOrdenados() []*Lead {
	lista := make([]*Lead, 0, len(a.leads))
	for _, l := range a.leads {
		lista = append(lista, l)
	}
	sort.Slice(lista, func(i, j int) bool {
		if lista[i].UltimaMsg.Equal(lista[j].UltimaMsg) {
			return lista[i].ID < lista[j].ID
		}
		return lista[i].UltimaMsg.After(lista[j].UltimaMsg)
	})
	return lista
}

// leadDe acha ou cria o lead de uma conversa. Chame com a.mu travado.
func (a *App) leadDe(id, telefone, nomeWhats string) *Lead {
	l := a.leads[id]
	if l == nil {
		l = &Lead{
			ID:            id,
			Classificacao: "indefinido",
			Status:        statusQualificando,
			CriadoEm:      time.Now(),
		}
		a.leads[id] = l
	}
	if telefone != "" {
		l.Telefone = telefone
	}
	if nomeWhats != "" {
		l.NomeWhats = nomeWhats
	}
	return l
}

// contatoDe acha ou cria o controle de ritmo. Chame com a.mu travado.
func (a *App) contatoDe(id string) *contato {
	c := a.contatos[id]
	if c == nil {
		c = &contato{}
		a.contatos[id] = c
	}
	return c
}

// anexar coloca uma mensagem no historico, sem deixar crescer demais.
func (l *Lead) anexar(m Msg) {
	l.Historico = append(l.Historico, m)
	if len(l.Historico) > maxHistorico {
		l.Historico = append([]Msg(nil), l.Historico[len(l.Historico)-maxHistorico:]...)
	}
}

// soltarPendentes tira a marca de "esperando resposta" de todas as mensagens.
func (l *Lead) soltarPendentes() {
	for i := range l.Historico {
		l.Historico[i].Pendente = false
	}
}

// retomar devolve o lead ao assistente. Chame com a.mu travado.
func (a *App) retomar(l *Lead) {
	l.Retomado = true
	l.Motivo = ""
	l.Atendente = ""
	if l.Classificacao != "" && l.Classificacao != "indefinido" {
		l.Status = statusQualificado
	} else {
		l.Status = statusQualificando
	}
	l.soltarPendentes()
	if c := a.contatos[l.ID]; c != nil {
		c.janela = nil
		c.avisouLimite = false
	}
}

// assumir passa a conversa para uma pessoa: o assistente para de responder nela.
// Chame com a.mu travado.
func (a *App) assumir(l *Lead, motivo string) {
	l.Status = statusHumano
	l.Motivo = motivo
	l.soltarPendentes()
}

// iniciarAtendimento registra que uma pessoa da equipe assumiu a conversa. O nome
// fica so no historico do painel: o cliente nunca ve. Chame com a.mu travado.
func (a *App) iniciarAtendimento(l *Lead, quem string) {
	l.Status = statusAtendimento
	l.Atendente = quem
	l.soltarPendentes()
	l.evento("assumiu", ouPadrao(quem, "A equipe")+" iniciou o atendimento")
}

// comHumano diz se a conversa esta com a equipe (na fila ou em atendimento).
// Nesses casos o assistente fica quieto.
func (l *Lead) comHumano() bool {
	return l.Status == statusHumano || l.Status == statusAtendimento
}

// evento guarda uma linha de historico interno. O cliente nao recebe.
func (l *Lead) evento(tipo, texto string) {
	l.anexar(Msg{De: "sistema", Tipo: "evento", Evento: tipo, Texto: texto, Hora: time.Now()})
}

// ehHumano diz se o telefone e um dos que recebem os avisos.
func (c Config) ehHumano(tel string) bool {
	for _, n := range c.NumerosHumanos {
		if mesmoNumero(tel, n) {
			return true
		}
	}
	return false
}

func (a *App) definirStatus(status, mensagem string) {
	a.mu.Lock()
	a.status = status
	a.mensagem = mensagem
	a.mu.Unlock()
}

func (a *App) definirErro(erro string) {
	a.mu.Lock()
	a.erro = erro
	a.mu.Unlock()
	if erro != "" {
		log.Printf("AVISO: %s", erro)
	}
}

func existeArquivo(p string) bool {
	st, err := os.Stat(p)
	return err == nil && !st.IsDir()
}

func ouPadrao(v, padrao string) string {
	if strings.TrimSpace(v) == "" {
		return padrao
	}
	return strings.TrimSpace(v)
}

// soDigitos tira tudo que nao for numero.
func soDigitos(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// chaveNumero normaliza um telefone para comparar. No Brasil o WhatsApp
// as vezes guarda o celular sem o nono digito, entao tiramos ele dos dois lados.
func chaveNumero(s string) string {
	d := soDigitos(s)
	if len(d) == 10 || len(d) == 11 { // numero brasileiro sem o 55
		d = "55" + d
	}
	if strings.HasPrefix(d, "55") && len(d) == 13 && d[4] == '9' {
		return d[:4] + d[5:]
	}
	return d
}

func mesmoNumero(a, b string) bool {
	ka, kb := chaveNumero(a), chaveNumero(b)
	return ka != "" && ka == kb
}

// telefoneBonito deixa o numero facil de ler: +55 27 99999-9999.
func telefoneBonito(s string) string {
	d := soDigitos(s)
	if strings.HasPrefix(d, "55") && (len(d) == 12 || len(d) == 13) {
		resto := d[4:]
		return "+55 " + d[2:4] + " " + resto[:len(resto)-4] + "-" + resto[len(resto)-4:]
	}
	if len(d) == 10 || len(d) == 11 { // numero brasileiro digitado sem o 55
		return "(" + d[:2] + ") " + d[2:len(d)-4] + "-" + d[len(d)-4:]
	}
	if d == "" {
		return s
	}
	return "+" + d
}

func cortar(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max])
}

func umaLinha(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// fimDoNumero devolve so os 4 ultimos digitos, para o log nao expor telefones.
func fimDoNumero(s string) string {
	d := soDigitos(s)
	if len(d) <= 4 {
		return "****"
	}
	return "****" + d[len(d)-4:]
}

var trocaAcentos = strings.NewReplacer(
	"á", "a", "à", "a", "â", "a", "ã", "a", "ä", "a",
	"é", "e", "è", "e", "ê", "e", "ë", "e",
	"í", "i", "ì", "i", "î", "i", "ï", "i",
	"ó", "o", "ò", "o", "ô", "o", "õ", "o", "ö", "o",
	"ú", "u", "ù", "u", "û", "u", "ü", "u",
	"ç", "c", "ñ", "n",
	"Á", "A", "À", "A", "Â", "A", "Ã", "A", "Ä", "A",
	"É", "E", "È", "E", "Ê", "E", "Ë", "E",
	"Í", "I", "Ì", "I", "Î", "I", "Ï", "I",
	"Ó", "O", "Ò", "O", "Ô", "O", "Õ", "O", "Ö", "O",
	"Ú", "U", "Ù", "U", "Û", "U", "Ü", "U",
	"Ç", "C", "Ñ", "N",
)

// semAcento troca letras acentuadas pelas simples, para comparar e buscar textos.
func semAcento(s string) string { return trocaAcentos.Replace(s) }

var diasSemana = [7]string{"domingo", "segunda-feira", "terça-feira", "quarta-feira", "quinta-feira", "sexta-feira", "sábado"}
var diasCurtos = [7]string{"dom", "seg", "ter", "qua", "qui", "sex", "sáb"}

// dataPorExtenso: "quarta-feira, 12/11/2026, 14:00".
func dataPorExtenso(t time.Time) string {
	return diasSemana[int(t.Weekday())] + ", " + t.Format("02/01/2006, 15:04")
}

// diaCurto: "qua 12/11".
func diaCurto(t time.Time) string {
	return diasCurtos[int(t.Weekday())] + " " + t.Format("02/01")
}

func novoID(prefixo string) string {
	b := make([]byte, 5)
	if _, err := crand.Read(b); err != nil {
		return prefixo + strconv.FormatInt(time.Now().UnixNano(), 36)
	}
	return prefixo + hex.EncodeToString(b)
}
