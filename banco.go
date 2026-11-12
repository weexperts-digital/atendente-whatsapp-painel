package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

// O banco de dados e um arquivo so (SQLite), dentro de dados/. Nao precisa instalar
// nem configurar nada: o programa cria na primeira vez. Nele ficam os dados do
// negocio, os contatos, TODAS as mensagens, a agenda, os usuarios do painel, as
// sessoes de login e o registro de quem fez o que.
var (
	arqBanco    = filepath.Join(pastaDados, "atendente.db")
	pastaCopias = filepath.Join(pastaDados, "copias")
)

const esquema = `
CREATE TABLE IF NOT EXISTS meta (
	chave TEXT PRIMARY KEY,
	valor TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS config (
	id          INTEGER PRIMARY KEY CHECK (id = 1),
	dados       TEXT NOT NULL,
	alterado_em TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS contatos (
	id            TEXT PRIMARY KEY,
	telefone      TEXT NOT NULL DEFAULT '',
	nome          TEXT NOT NULL DEFAULT '',
	nome_whatsapp TEXT NOT NULL DEFAULT '',
	situacao      TEXT NOT NULL DEFAULT '',
	temperatura   TEXT NOT NULL DEFAULT '',
	pontuacao     INTEGER NOT NULL DEFAULT 0,
	etiquetas     TEXT NOT NULL DEFAULT '[]',
	atendente     TEXT NOT NULL DEFAULT '',
	criado_em     TEXT NOT NULL DEFAULT '',
	ultima_msg    TEXT NOT NULL DEFAULT '',
	dados         TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS mensagens (
	n           INTEGER PRIMARY KEY AUTOINCREMENT,
	contato_id  TEXT NOT NULL REFERENCES contatos(id) ON DELETE CASCADE,
	de          TEXT NOT NULL,
	texto       TEXT NOT NULL DEFAULT '',
	hora        TEXT NOT NULL,
	tipo        TEXT NOT NULL DEFAULT '',
	midia       TEXT NOT NULL DEFAULT '',
	arquivo     TEXT NOT NULL DEFAULT '',
	mime        TEXT NOT NULL DEFAULT '',
	tamanho     INTEGER NOT NULL DEFAULT 0,
	autor       TEXT NOT NULL DEFAULT '',
	evento      TEXT NOT NULL DEFAULT '',
	id_whatsapp TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS mensagens_por_contato ON mensagens (contato_id, n);

CREATE TABLE IF NOT EXISTS agendamentos (
	id          TEXT PRIMARY KEY,
	contato_id  TEXT NOT NULL DEFAULT '',
	nome        TEXT NOT NULL DEFAULT '',
	telefone    TEXT NOT NULL DEFAULT '',
	servico     TEXT NOT NULL DEFAULT '',
	inicio      TEXT NOT NULL,
	duracao_min INTEGER NOT NULL DEFAULT 0,
	origem      TEXT NOT NULL DEFAULT '',
	status      TEXT NOT NULL DEFAULT '',
	obs         TEXT NOT NULL DEFAULT '',
	criado_em   TEXT NOT NULL DEFAULT ''
);

CREATE TABLE IF NOT EXISTS usuarios (
	id            TEXT PRIMARY KEY,
	nome          TEXT NOT NULL,
	email         TEXT NOT NULL UNIQUE,
	papel         TEXT NOT NULL,
	senha         TEXT NOT NULL,
	ativo         INTEGER NOT NULL DEFAULT 1,
	trocar_senha  INTEGER NOT NULL DEFAULT 0,
	criado_em     TEXT NOT NULL,
	ultimo_acesso TEXT NOT NULL DEFAULT ''
);

CREATE TABLE IF NOT EXISTS sessoes (
	chave      TEXT PRIMARY KEY,
	usuario_id TEXT NOT NULL REFERENCES usuarios(id) ON DELETE CASCADE,
	criada_em  TEXT NOT NULL,
	vista_em   TEXT NOT NULL,
	navegador  TEXT NOT NULL DEFAULT ''
);

CREATE TABLE IF NOT EXISTS registro (
	n          INTEGER PRIMARY KEY AUTOINCREMENT,
	quando     TEXT NOT NULL,
	usuario_id TEXT NOT NULL DEFAULT '',
	usuario    TEXT NOT NULL DEFAULT '',
	acao       TEXT NOT NULL,
	detalhe    TEXT NOT NULL DEFAULT ''
);
`

// copiaLead guarda o que ja esta gravado de um contato, para gravar so o que mudou.
type copiaLead struct {
	dados string
	msgs  map[int64]string
}

type Banco struct {
	db     *sql.DB
	mu     sync.Mutex // uma gravacao por vez
	leads  map[string]*copiaLead
	agenda map[string]string
}

// formatoFixo deixa as datas com o mesmo tamanho, para a ordem alfabetica ser a ordem do tempo.
const formatoFixo = "2006-01-02T15:04:05.000Z"

func horaFixa(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(formatoFixo)
}

func horaExata(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format(time.RFC3339Nano)
}

func lerHora(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return time.Time{}
	}
	return t
}

func abrirBanco(caminho string) (*Banco, error) {
	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(caminho)+"?_pragma=foreign_keys(1)&_pragma=busy_timeout(10000)&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)")
	if err != nil {
		return nil, err
	}
	if _, err := db.Exec(esquema); err != nil {
		db.Close()
		return nil, fmt.Errorf("criando as tabelas: %w", err)
	}
	_ = os.Chmod(caminho, 0o600)
	b := &Banco{db: db, leads: map[string]*copiaLead{}, agenda: map[string]string{}}
	if b.meta("esquema") == "" {
		if err := b.definirMeta("esquema", "1"); err != nil {
			db.Close()
			return nil, err
		}
	}
	return b, nil
}

func (b *Banco) fechar() {
	if b != nil && b.db != nil {
		_ = b.db.Close()
	}
}

func (b *Banco) meta(chave string) string {
	var v string
	_ = b.db.QueryRow(`SELECT valor FROM meta WHERE chave = ?`, chave).Scan(&v)
	return v
}

// definirMeta guarda um valor solto. Valor vazio apaga a chave.
func (b *Banco) definirMeta(chave, valor string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if valor == "" {
		_, err := b.db.Exec(`DELETE FROM meta WHERE chave = ?`, chave)
		return err
	}
	_, err := b.db.Exec(`INSERT INTO meta (chave, valor) VALUES (?, ?) ON CONFLICT(chave) DO UPDATE SET valor = excluded.valor`, chave, valor)
	return err
}

// ---------- dados do negocio ----------

func (b *Banco) lerConfig() (Config, bool, error) {
	var c Config
	var dados string
	err := b.db.QueryRow(`SELECT dados FROM config WHERE id = 1`).Scan(&dados)
	if err == sql.ErrNoRows {
		return c, false, nil
	}
	if err != nil {
		return c, false, err
	}
	if err := json.Unmarshal([]byte(dados), &c); err != nil {
		return Config{}, false, err
	}
	return c, true, nil
}

func (b *Banco) gravarConfig(c Config) error {
	dados, err := json.Marshal(c)
	if err != nil {
		return err
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	_, err = b.db.Exec(`INSERT INTO config (id, dados, alterado_em) VALUES (1, ?, ?)
		ON CONFLICT(id) DO UPDATE SET dados = excluded.dados, alterado_em = excluded.alterado_em`, string(dados), horaFixa(time.Now()))
	return err
}

// ---------- contatos e mensagens ----------

// dadosDoLead e o contato inteiro em JSON, sem as mensagens (elas tem tabela propria).
func dadosDoLead(l *Lead) (string, error) {
	c := *l
	c.Historico = nil
	b, err := json.Marshal(&c)
	return string(b), err
}

// assinatura muda quando qualquer parte gravada da mensagem muda.
func assinatura(m *Msg) string {
	return strings.Join([]string{m.De, m.Tipo, m.Evento, m.Midia, m.Nome, m.Mime, strconv.Itoa(m.Tamanho), m.Autor, m.ID, horaExata(m.Hora), m.Texto}, "\x1f")
}

// gravarLeads leva para o banco o que mudou desde a ultima gravacao: contatos novos
// ou alterados, mensagens novas ou alteradas e contatos apagados. As mensagens que
// saem da memoria (o historico em uso e cortado) continuam guardadas no banco.
func (b *Banco) gravarLeads(leads map[string]*Lead) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	tx, err := b.db.Begin()
	if err != nil {
		return err
	}
	var numeradas []*Msg
	desfazer := func(err error) error {
		_ = tx.Rollback()
		for _, m := range numeradas {
			m.N = 0
		}
		return err
	}
	novas := make(map[string]*copiaLead, len(leads))
	for id, l := range leads {
		ant := b.leads[id]
		dados, err := dadosDoLead(l)
		if err != nil {
			return desfazer(err)
		}
		if ant == nil || ant.dados != dados {
			etiquetas := l.Etiquetas
			if etiquetas == nil {
				etiquetas = []string{}
			}
			etq, _ := json.Marshal(etiquetas)
			_, err = tx.Exec(`INSERT INTO contatos (id, telefone, nome, nome_whatsapp, situacao, temperatura, pontuacao, etiquetas, atendente, criado_em, ultima_msg, dados)
				VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
				ON CONFLICT(id) DO UPDATE SET telefone = excluded.telefone, nome = excluded.nome, nome_whatsapp = excluded.nome_whatsapp,
					situacao = excluded.situacao, temperatura = excluded.temperatura, pontuacao = excluded.pontuacao, etiquetas = excluded.etiquetas,
					atendente = excluded.atendente, criado_em = excluded.criado_em, ultima_msg = excluded.ultima_msg, dados = excluded.dados`,
				id, soDigitos(l.Telefone), l.Nome, l.NomeWhats, situacaoDe(l), ouPadrao(l.Classificacao, "indefinido"), l.Pontuacao,
				string(etq), l.Atendente, horaFixa(l.CriadoEm), horaFixa(l.UltimaMsg), dados)
			if err != nil {
				return desfazer(err)
			}
		}
		nova := &copiaLead{dados: dados, msgs: make(map[int64]string, len(l.Historico))}
		for i := range l.Historico {
			m := &l.Historico[i]
			ass := assinatura(m)
			switch {
			case m.N == 0:
				res, err := tx.Exec(`INSERT INTO mensagens (contato_id, de, texto, hora, tipo, midia, arquivo, mime, tamanho, autor, evento, id_whatsapp)
					VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
					id, m.De, m.Texto, horaExata(m.Hora), m.Tipo, m.Midia, m.Nome, m.Mime, m.Tamanho, m.Autor, m.Evento, m.ID)
				if err != nil {
					return desfazer(err)
				}
				n, err := res.LastInsertId()
				if err != nil {
					return desfazer(err)
				}
				m.N = n
				numeradas = append(numeradas, m)
			case ant == nil || ant.msgs[m.N] != ass:
				_, err := tx.Exec(`UPDATE mensagens SET de = ?, texto = ?, hora = ?, tipo = ?, midia = ?, arquivo = ?, mime = ?, tamanho = ?, autor = ?, evento = ?, id_whatsapp = ?
					WHERE n = ? AND contato_id = ?`,
					m.De, m.Texto, horaExata(m.Hora), m.Tipo, m.Midia, m.Nome, m.Mime, m.Tamanho, m.Autor, m.Evento, m.ID, m.N, id)
				if err != nil {
					return desfazer(err)
				}
			}
			nova.msgs[m.N] = ass
		}
		novas[id] = nova
	}
	for id := range b.leads {
		if leads[id] == nil { // contato apagado no painel: as mensagens vao junto
			if _, err := tx.Exec(`DELETE FROM contatos WHERE id = ?`, id); err != nil {
				return desfazer(err)
			}
		}
	}
	if err := tx.Commit(); err != nil {
		return desfazer(err)
	}
	b.leads = novas
	return nil
}

const colunasMsg = `n, de, texto, hora, tipo, midia, arquivo, mime, tamanho, autor, evento, id_whatsapp`

func lerMsg(ler func(dest ...any) error, extras ...any) (Msg, error) {
	var m Msg
	var hora string
	dest := append([]any{&m.N, &m.De, &m.Texto, &hora, &m.Tipo, &m.Midia, &m.Nome, &m.Mime, &m.Tamanho, &m.Autor, &m.Evento, &m.ID}, extras...)
	if err := ler(dest...); err != nil {
		return m, err
	}
	m.Hora = lerHora(hora)
	return m, nil
}

// lerLeads devolve os contatos com as ultimas mensagens de cada um (ate maxPorLead).
func (b *Banco) lerLeads(maxPorLead int) ([]*Lead, error) {
	linhas, err := b.db.Query(`SELECT id, dados FROM contatos`)
	if err != nil {
		return nil, err
	}
	var lista []*Lead
	porID := map[string]*Lead{}
	for linhas.Next() {
		var id, dados string
		if err := linhas.Scan(&id, &dados); err != nil {
			linhas.Close()
			return nil, err
		}
		l := &Lead{}
		if err := json.Unmarshal([]byte(dados), l); err != nil {
			log.Printf("AVISO: contato %s com dados quebrados no banco, ignorando: %v", fimDoNumero(id), err)
			continue
		}
		l.ID = id
		l.Historico = nil
		lista = append(lista, l)
		porID[id] = l
	}
	linhas.Close()
	if err := linhas.Err(); err != nil {
		return nil, err
	}

	linhas, err = b.db.Query(`SELECT `+colunasMsg+`, contato_id FROM (
			SELECT *, ROW_NUMBER() OVER (PARTITION BY contato_id ORDER BY n DESC) AS ordem FROM mensagens
		) WHERE ordem <= ? ORDER BY contato_id, n`, maxPorLead)
	if err != nil {
		return nil, err
	}
	for linhas.Next() {
		var contato string
		m, err := lerMsg(linhas.Scan, &contato)
		if err != nil {
			linhas.Close()
			return nil, err
		}
		if l := porID[contato]; l != nil {
			l.Historico = append(l.Historico, m)
		}
	}
	linhas.Close()
	if err := linhas.Err(); err != nil {
		return nil, err
	}

	b.mu.Lock()
	b.leads = make(map[string]*copiaLead, len(lista))
	for _, l := range lista {
		dados, _ := dadosDoLead(l)
		c := &copiaLead{dados: dados, msgs: make(map[int64]string, len(l.Historico))}
		for i := range l.Historico {
			c.msgs[l.Historico[i].N] = assinatura(&l.Historico[i])
		}
		b.leads[l.ID] = c
	}
	b.mu.Unlock()
	return lista, nil
}

// temAntes diz se o contato tem mensagens gravadas antes da de numero n.
func (b *Banco) temAntes(contato string, n int64) bool {
	var tem bool
	_ = b.db.QueryRow(`SELECT EXISTS (SELECT 1 FROM mensagens WHERE contato_id = ? AND n < ?)`, contato, n).Scan(&tem)
	return tem
}

// mensagensAntes devolve mensagens mais antigas que a de numero "antes", em ordem de
// tempo. Serve para a tela mostrar o comeco de conversas longas.
func (b *Banco) mensagensAntes(contato string, antes int64, limite int) ([]Msg, bool, error) {
	linhas, err := b.db.Query(`SELECT `+colunasMsg+` FROM mensagens WHERE contato_id = ? AND n < ? ORDER BY n DESC LIMIT ?`, contato, antes, limite+1)
	if err != nil {
		return nil, false, err
	}
	defer linhas.Close()
	var msgs []Msg
	for linhas.Next() {
		m, err := lerMsg(linhas.Scan)
		if err != nil {
			return nil, false, err
		}
		msgs = append(msgs, m)
	}
	temMais := len(msgs) > limite
	if temMais {
		msgs = msgs[:limite]
	}
	sort.Slice(msgs, func(i, j int) bool { return msgs[i].N < msgs[j].N })
	return msgs, temMais, linhas.Err()
}

// ---------- agenda ----------

func (b *Banco) gravarAgenda(lista []*Agendamento) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	tx, err := b.db.Begin()
	if err != nil {
		return err
	}
	novas := make(map[string]string, len(lista))
	for _, ag := range lista {
		j, err := json.Marshal(ag)
		if err != nil {
			_ = tx.Rollback()
			return err
		}
		novas[ag.ID] = string(j)
		if b.agenda[ag.ID] == string(j) {
			continue
		}
		_, err = tx.Exec(`INSERT INTO agendamentos (id, contato_id, nome, telefone, servico, inicio, duracao_min, origem, status, obs, criado_em)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(id) DO UPDATE SET contato_id = excluded.contato_id, nome = excluded.nome, telefone = excluded.telefone, servico = excluded.servico,
				inicio = excluded.inicio, duracao_min = excluded.duracao_min, origem = excluded.origem, status = excluded.status, obs = excluded.obs, criado_em = excluded.criado_em`,
			ag.ID, ag.LeadID, ag.Nome, ag.Telefone, ag.Servico, horaExata(ag.Inicio), ag.DuracaoMin, ag.Origem, ag.Status, ag.Obs, horaExata(ag.CriadoEm))
		if err != nil {
			_ = tx.Rollback()
			return err
		}
	}
	for id := range b.agenda {
		if _, existe := novas[id]; !existe {
			if _, err := tx.Exec(`DELETE FROM agendamentos WHERE id = ?`, id); err != nil {
				_ = tx.Rollback()
				return err
			}
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	b.agenda = novas
	return nil
}

func (b *Banco) lerAgenda() ([]*Agendamento, error) {
	linhas, err := b.db.Query(`SELECT id, contato_id, nome, telefone, servico, inicio, duracao_min, origem, status, obs, criado_em FROM agendamentos ORDER BY inicio, id`)
	if err != nil {
		return nil, err
	}
	defer linhas.Close()
	var lista []*Agendamento
	copia := map[string]string{}
	for linhas.Next() {
		ag := &Agendamento{}
		var inicio, criado string
		if err := linhas.Scan(&ag.ID, &ag.LeadID, &ag.Nome, &ag.Telefone, &ag.Servico, &inicio, &ag.DuracaoMin, &ag.Origem, &ag.Status, &ag.Obs, &criado); err != nil {
			return nil, err
		}
		ag.Inicio, ag.CriadoEm = lerHora(inicio), lerHora(criado)
		lista = append(lista, ag)
		if j, err := json.Marshal(ag); err == nil {
			copia[ag.ID] = string(j)
		}
	}
	if err := linhas.Err(); err != nil {
		return nil, err
	}
	b.mu.Lock()
	b.agenda = copia
	b.mu.Unlock()
	return lista, nil
}

// limparConversas apaga contatos, mensagens e agenda. So e usado antes de trazer
// os arquivos antigos, para uma migracao interrompida nao deixar nada em dobro.
func (b *Banco) limparConversas() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, tabela := range []string{"mensagens", "contatos", "agendamentos"} {
		if _, err := b.db.Exec(`DELETE FROM ` + tabela); err != nil {
			return err
		}
	}
	b.leads = map[string]*copiaLead{}
	b.agenda = map[string]string{}
	return nil
}

// ---------- copia de seguranca ----------

// copiaDiaria guarda uma copia inteira do banco por dia em dados/copias e mantem
// so as 7 mais novas. Se o arquivo principal estragar, da para voltar a de ontem.
func (b *Banco) copiaDiaria(agora time.Time) {
	if err := os.MkdirAll(pastaCopias, 0o700); err != nil {
		log.Printf("AVISO: não criei a pasta de cópias: %v", err)
		return
	}
	destino := filepath.Join(pastaCopias, "atendente-"+agora.Format("2006-01-02")+".db")
	if !existeArquivo(destino) {
		b.mu.Lock()
		_, err := b.db.Exec(`VACUUM INTO ?`, filepath.ToSlash(destino))
		b.mu.Unlock()
		if err != nil {
			log.Printf("AVISO: não consegui fazer a cópia de segurança do banco: %v", err)
			return
		}
		_ = os.Chmod(destino, 0o600)
		log.Printf("BANCO: cópia de segurança de hoje guardada em %s", destino)
	}
	copias, _ := filepath.Glob(filepath.Join(pastaCopias, "atendente-*.db"))
	sort.Strings(copias)
	for len(copias) > 7 {
		_ = os.Remove(copias[0])
		copias = copias[1:]
	}
}
