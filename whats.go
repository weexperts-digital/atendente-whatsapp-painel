package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"time"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/store"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	waLog "go.mau.fi/whatsmeow/util/log"
	"google.golang.org/protobuf/proto"
	_ "modernc.org/sqlite"
)

const (
	msgPasskey   = "Este WhatsApp está exigindo uma verificação extra (passkey) que este programa ainda não consegue completar. Tente com outro número ou um WhatsApp Business, ou peça ajuda ao instrutor."
	msgSoTexto   = "Por enquanto eu só leio texto. Pode escrever sua dúvida?"
	msgUmaPorVez = "Vou responder uma de cada vez, só um instante"
)

// iniciarWhats abre a sessao (SQLite em Go puro) e comeca a conectar.
func (a *App) iniciarWhats(ctx context.Context) error {
	store.DeviceProps.Os = proto.String("Atendente IA")

	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(arqSessao)+"?_pragma=foreign_keys(1)&_pragma=busy_timeout(10000)&_pragma=journal_mode(WAL)")
	if err != nil {
		return err
	}
	db.SetMaxOpenConns(1)
	a.container = sqlstore.NewWithDB(db, "sqlite3", waLog.Stdout("Sessao", "WARN", false))
	if err := a.container.Upgrade(ctx); err != nil {
		return err
	}
	_ = os.Chmod(arqSessao, 0o600)
	go a.loopConexao(ctx)
	return nil
}

func dormir(ctx context.Context, d time.Duration) bool {
	select {
	case <-time.After(d):
		return true
	case <-ctx.Done():
		return false
	}
}

// pedirTroca avisa o loop de conexao que o aparelho saiu e e preciso parear de novo.
func (a *App) pedirTroca() {
	select {
	case a.trocar <- struct{}{}:
	default:
	}
}

// loopConexao mantem o WhatsApp ligado: pareia pelo QR, conecta e refaz tudo se o
// aparelho for removido no celular ou pelo painel.
func (a *App) loopConexao(ctx context.Context) {
	for ctx.Err() == nil {
		select {
		case <-a.trocar: // pedido antigo, ja atendido
		default:
		}
		dev, err := a.container.GetFirstDevice(ctx)
		if err != nil {
			a.definirErro("Não consegui abrir a sessão do WhatsApp: " + err.Error())
			if !dormir(ctx, 5*time.Second) {
				return
			}
			continue
		}
		cli := whatsmeow.NewClient(dev, waLog.Stdout("WhatsApp", "WARN", false))
		cli.AddEventHandler(func(evt any) { a.tratarEvento(cli, evt) })
		a.mu.Lock()
		a.cli = cli
		a.mu.Unlock()

		if cli.Store.ID == nil {
			if !a.parear(ctx, cli) {
				cli.Disconnect()
				if !dormir(ctx, 2*time.Second) {
					return
				}
				continue
			}
		} else {
			a.definirStatus("conectando", "Conectando ao WhatsApp...")
			for ctx.Err() == nil {
				if err := cli.Connect(); err == nil {
					break
				} else {
					log.Printf("WHATSAPP: não conectou: %v", err)
				}
				a.definirStatus("desconectado", "Desconectado. Sem conexão com o WhatsApp, tentando de novo sozinho...")
				if !dormir(ctx, 5*time.Second) {
					return
				}
			}
		}

		select {
		case <-a.trocar:
			cli.Disconnect()
			a.mu.Lock()
			a.nomeConta, a.numConta = "", ""
			a.mu.Unlock()
			dormir(ctx, 2*time.Second)
		case <-ctx.Done():
			cli.Disconnect()
			return
		}
	}
}

// parear mostra o QR ate o celular ler. Devolve verdadeiro quando pareou.
func (a *App) parear(ctx context.Context, cli *whatsmeow.Client) bool {
	qrChan, err := cli.GetQRChannel(ctx)
	if err != nil {
		a.definirErro("Não consegui preparar o código QR: " + err.Error())
		return false
	}
	if err := cli.Connect(); err != nil {
		log.Printf("WHATSAPP: não conectou para gerar o QR: %v", err)
		a.definirStatus("desconectado", "Desconectado. Sem conexão com o WhatsApp. Confira a internet, estou tentando de novo...")
		dormir(ctx, 4*time.Second)
		return false
	}
	pareado := false
	for item := range qrChan {
		switch item.Event {
		case whatsmeow.QRChannelEventCode:
			a.mu.Lock()
			a.qr = item.Code
			a.status = "aguardando"
			a.mensagem = "Aguardando leitura do código"
			a.mu.Unlock()
		case whatsmeow.QRChannelSuccess.Event:
			pareado = true
			a.mu.Lock()
			a.qr = ""
			a.erro = ""
			a.status = "conectando"
			a.mensagem = "Código lido. Conectando..."
			a.mu.Unlock()
			log.Printf("WHATSAPP: código lido, aparelho vinculado")
		case whatsmeow.QRChannelTimeout.Event:
			a.mu.Lock()
			a.qr = ""
			a.status = "aguardando"
			a.mensagem = "O código expirou. Gerando outro..."
			a.mu.Unlock()
		case whatsmeow.QRChannelScannedWithoutMultidevice.Event:
			a.definirErro("O celular leu o código, mas o recurso de vários aparelhos está desligado. Atualize o WhatsApp do celular e leia o código de novo.")
		case whatsmeow.QRChannelClientOutdated.Event:
			a.definirErro("O WhatsApp recusou esta versão do programa. Abra o Claude Code nesta pasta e peça: atualize o atendente.")
		case whatsmeow.QRChannelEventPasskeyRequest, whatsmeow.QRChannelEventPasskeyResponse:
			a.definirErro(msgPasskey)
		case whatsmeow.QRChannelEventError:
			if item.Error != nil && strings.Contains(strings.ToLower(item.Error.Error()), "passkey") {
				a.definirErro(msgPasskey)
			} else {
				a.definirErro(fmt.Sprintf("Não foi possível vincular o aparelho. Tente ler o código de novo. Detalhe: %v", item.Error))
			}
		default:
			log.Printf("WHATSAPP: evento de pareamento: %s", item.Event)
		}
	}
	a.mu.Lock()
	a.qr = ""
	a.mu.Unlock()
	return pareado || cli.Store.ID != nil
}

func (a *App) tratarEvento(cli *whatsmeow.Client, evt any) {
	switch v := evt.(type) {
	case *events.Message:
		a.aoReceber(cli, v)
	case *events.UndecryptableMessage:
		// Comum logo depois de vincular o aparelho. A biblioteca pede a mensagem
		// de novo ao celular sozinha; aqui so fica o registro.
		if s := v.Info.Chat.Server; !v.Info.IsFromMe && !v.Info.IsGroup && (s == types.DefaultUserServer || s == types.HiddenUserServer) {
			log.Printf("WHATSAPP: uma mensagem de %s chegou fechada. Pedi de novo ao celular.", fimDoNumero(telefoneDe(cli, v.Info)))
		}
	case *events.Connected:
		a.atualizarConta(cli, true)
		log.Printf("WHATSAPP: conectado")
		go a.atualizarFotos(cli)
	case *events.PushNameSetting, *events.AppStateSyncComplete:
		a.atualizarConta(cli, false)
	case *events.Disconnected:
		a.mu.Lock()
		if a.status == "conectado" || a.status == "conectando" {
			a.status = "desconectado"
			a.mensagem = "Desconectado. A conexão caiu, reconectando sozinho..."
		}
		a.mu.Unlock()
	case *events.LoggedOut:
		a.definirStatus("desconectado", "Desconectado. Este aparelho foi removido no celular. Leia o código de novo.")
		log.Printf("WHATSAPP: aparelho removido no celular")
		a.pedirTroca()
	case *events.StreamReplaced:
		a.definirErro("Esta mesma sessão foi aberta em outro programa ao mesmo tempo. Feche o outro e ligue o atendente de novo.")
	case *events.TemporaryBan:
		a.definirErro("O WhatsApp bloqueou este número por um tempo. Aguarde e tente mais tarde. Detalhe: " + v.String())
	case *events.ClientOutdated:
		a.definirErro("O WhatsApp recusou esta versão do programa. Abra o Claude Code nesta pasta e peça: atualize o atendente.")
	case *events.PairPasskeyRequest, *events.PairPasskeyError:
		a.definirErro(msgPasskey)
	}
}

// atualizarConta guarda o nome e o numero da conta conectada.
func (a *App) atualizarConta(cli *whatsmeow.Client, conectou bool) {
	nome := cli.Store.PushName
	if nome == "" {
		nome = cli.Store.BusinessName
	}
	num := ""
	if cli.Store.ID != nil {
		num = cli.Store.ID.User
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.cli != cli {
		return
	}
	if conectou {
		a.status = "conectado"
		a.qr = ""
		a.erro = ""
	}
	a.nomeConta, a.numConta = nome, num
	if a.status == "conectado" {
		if nome != "" {
			a.mensagem = "Conectado como " + nome
		} else {
			a.mensagem = "Conectado como " + telefoneBonito(num)
		}
	}
}

func (a *App) cliente() *whatsmeow.Client {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.cli
}

// telefoneDoLID troca o endereco escondido (@lid) pelo telefone, se a sessao souber.
func telefoneDoLID(cli *whatsmeow.Client, lid types.JID) string {
	ctx, cancelar := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancelar()
	if pn, err := cli.Store.LIDs.GetPNForLID(ctx, lid.ToNonAD()); err == nil && !pn.IsEmpty() {
		return pn.User
	}
	return ""
}

// telefoneDe descobre o telefone de quem escreveu, mesmo quando o WhatsApp manda so o @lid.
func telefoneDe(cli *whatsmeow.Client, info types.MessageInfo) string {
	rem := info.Sender
	if rem.Server == types.HiddenUserServer {
		if !info.SenderAlt.IsEmpty() && info.SenderAlt.Server == types.DefaultUserServer {
			return info.SenderAlt.User
		}
		if tel := telefoneDoLID(cli, rem); tel != "" {
			return tel
		}
	}
	return rem.User
}

// oQueFazer diz o destino de uma mensagem privada.
type oQueFazer int

const (
	fazConversa oQueFazer = iota // segue para o assistente
	fazComando                   // /lista ou /retomar, vindo do numero humano
	fazNada                      // nao responde e nao guarda
)

// decidir escolhe o destino da mensagem pelo numero de quem escreveu.
// O numero humano conversa com o assistente como qualquer pessoa (e assim que o
// dono testa). So os comandos dele, que comecam com barra, tem tratamento especial.
func decidir(cfg Config, tel, texto string) oQueFazer {
	if cfg.ehHumano(tel) && strings.HasPrefix(texto, "/") {
		return fazComando
	}
	for _, n := range cfg.NuncaResponder {
		if mesmoNumero(tel, n) {
			return fazNada
		}
	}
	return fazConversa
}

// aoReceber trata cada mensagem que chega. So conversas privadas, so de quem escreveu.
func (a *App) aoReceber(cli *whatsmeow.Client, evt *events.Message) {
	info := evt.Info
	if info.IsGroup || evt.IsEdit || evt.Message == nil {
		return
	}
	chat := info.Chat.ToNonAD()
	// Fora: grupos, status, canais (newsletter), listas de transmissao e contas do sistema.
	if chat.Server != types.DefaultUserServer && chat.Server != types.HiddenUserServer {
		return
	}
	if chat.User == "" || chat.User == "0" || chat == types.MetaAIJID || info.IsIncomingBroadcast() || evt.NewsletterMeta != nil {
		return
	}

	m := evt.Message
	texto := strings.TrimSpace(m.GetConversation())
	if texto == "" {
		texto = strings.TrimSpace(m.GetExtendedTextMessage().GetText())
	}
	midia := midiaDe(m)
	if texto == "" && midia == nil {
		return // reacoes, confirmacoes e outros avisos internos
	}
	// Midia com legenda: a legenda vale como o texto da mensagem. Sem legenda, a IA
	// nao tem o que ler (ela so le texto); a midia fica guardada para a equipe ver.
	soMidia := false
	if midia != nil {
		texto = strings.TrimSpace(midia.Legenda)
		soMidia = texto == ""
	}
	msg := Msg{De: "cliente", Texto: cortar(texto, maxTextoMsg), ID: string(info.ID)}
	if midia != nil {
		if midia.Tipo == "outro" {
			msg.Texto = midia.rotulo()
		} else {
			msg.Tipo, msg.Nome, msg.Mime, msg.Tamanho = midia.Tipo, midia.Nome, mimeLimpo(midia.Mime), midia.Tamanho
		}
	}

	if info.IsFromMe {
		// A pessoa do negocio respondeu pelo proprio celular.
		a.anotarRespostaDoCelular(cli, info, chat, msg, midia)
		return
	}

	tel := telefoneDe(cli, info)
	id := chat.String()
	agora := time.Now()
	msg.Hora = agora
	antiga := !info.Timestamp.IsZero() && agora.Sub(info.Timestamp) > idadeMaximaMsg

	a.mu.Lock()
	cfg := a.cfg

	switch decidir(cfg, tel, msg.Texto) {
	case fazComando:
		a.mu.Unlock()
		if antiga {
			log.Printf("COMANDO: ignorado por ser antigo")
			return
		}
		go a.comandoHumano(chat, msg.Texto)
		return
	case fazNada:
		a.mu.Unlock()
		log.Printf("MENSAGEM: ignorada (número na lista de nunca responder)")
		return
	}

	l := a.leadDe(id, tel, info.PushName)
	l.UltimaMsg = agora
	podeResponder := cfg.Ligado && cfg.Salvo && !l.comHumano() && !antiga
	buscarFoto := time.Since(l.FotoEm) > 24*time.Hour
	depois := func() {
		if midia != nil {
			go a.baixarMidia(cli, id, info.ID, midia)
		}
		if buscarFoto {
			go a.atualizarFoto(cli, id)
		}
	}

	if soMidia {
		l.anexar(msg)
		// Audio: o atendente ouve (transcreve neste computador) e responde. O aviso de
		// "so leio texto" fica para quando nao der para ouvir.
		// Mensagem antiga (chegou atrasada) nao e ouvida nem respondida.
		ouvir := msg.Tipo == "audio" && midia != nil && !antiga && ouvidoDoPainel.disponivel()
		avisar := podeResponder && !l.AvisouMidia && !ouvir
		if avisar {
			l.AvisouMidia = true
		}
		a.salvarLeads()
		a.mu.Unlock()
		log.Printf("MENSAGEM: de %s (mídia sem texto, responder=%v, ouvir=%v)", fimDoNumero(tel), podeResponder, ouvir)
		if ouvir {
			go func() {
				a.baixarMidia(cli, id, info.ID, midia)
				a.ouvirMensagem(cli, info, chat, id)
			}()
			if buscarFoto {
				go a.atualizarFoto(cli, id)
			}
			return
		}
		depois()
		if avisar {
			go func() {
				a.marcarLida(cli, info)
				time.Sleep(time.Duration(2000+rand.Intn(2001)) * time.Millisecond)
				a.enviarEAnotar(id, chat, msgSoTexto)
			}()
		}
		return
	}

	iniciar, avisarLimite := false, false
	if podeResponder {
		podeResponder, iniciar, avisarLimite = a.entrarNaVez(id, cfg, agora)
		msg.Pendente = podeResponder
	}
	l.anexar(msg)
	a.salvarLeads()
	a.mu.Unlock()

	log.Printf("MENSAGEM: de %s (responder=%v)", fimDoNumero(tel), podeResponder)
	depois()

	if avisarLimite {
		go a.enviarEAnotar(id, chat, msgUmaPorVez)
	}
	if podeResponder {
		// So marca como lida quando o agente vai responder. Se esta pausado ou
		// com a equipe, fica como nao lida para a pessoa ver no celular.
		go a.marcarLida(cli, info)
	}
	if iniciar {
		go a.trabalhar(id)
	}
}

// entrarNaVez aplica o limite de mensagens por contato e diz se esta mensagem vai ser
// respondida, se e preciso iniciar o trabalho de responder e se cabe o aviso de limite.
// Chame com a.mu travado.
func (a *App) entrarNaVez(id string, cfg Config, agora time.Time) (responder, iniciar, avisarLimite bool) {
	c := a.contatoDe(id)
	// Limite de mensagens processadas a cada 10 minutos por contato.
	validas := c.janela[:0]
	for _, t := range c.janela {
		if agora.Sub(t) < janelaLimite {
			validas = append(validas, t)
		}
	}
	c.janela = validas
	if len(c.janela) >= cfg.LimiteMsgs {
		if !c.avisouLimite {
			c.avisouLimite = true
			avisarLimite = true
		}
		return false, false, avisarLimite
	}
	c.janela = append(c.janela, agora)
	c.avisouLimite = false
	if !c.trabalhando {
		c.trabalhando = true
		iniciar = true
	}
	return true, iniciar, false
}

// ouvirMensagem transcreve o audio que o cliente mandou (ja baixado) e trata o texto
// como se a pessoa tivesse escrito: guarda na mensagem, para a equipe ler no painel, e
// poe o assistente para responder. Se nao der para ouvir, vale o aviso de "so leio texto".
func (a *App) ouvirMensagem(cli *whatsmeow.Client, info types.MessageInfo, chat types.JID, id string) {
	achar := func(l *Lead) *Msg {
		if l == nil {
			return nil
		}
		for i := len(l.Historico) - 1; i >= 0; i-- {
			if l.Historico[i].ID == string(info.ID) {
				return &l.Historico[i]
			}
		}
		return nil
	}
	a.mu.Lock()
	arquivo := ""
	if m := achar(a.leads[id]); m != nil {
		arquivo = m.Midia
	}
	a.mu.Unlock()

	texto, cortado, err := "", false, errSemOuvido
	if arquivo != "" {
		texto, cortado, err = a.ouvirArquivo(arquivo)
	}
	if err != nil {
		log.Printf("OUVIDO: não transcrevi o áudio de %s: %v", fimDoNumero(id), err)
	}

	agora := time.Now()
	a.mu.Lock()
	l := a.leads[id]
	if l == nil {
		a.mu.Unlock()
		return
	}
	cfg := a.cfg
	podeResponder := cfg.Ligado && cfg.Salvo && !l.comHumano()
	m := achar(l)
	if err != nil || texto == "" || m == nil {
		avisar := podeResponder && !l.AvisouMidia
		if avisar {
			l.AvisouMidia = true
			a.salvarLeads()
		}
		a.mu.Unlock()
		if avisar {
			a.marcarLida(cli, info)
			time.Sleep(time.Duration(1500+rand.Intn(1501)) * time.Millisecond)
			a.enviarEAnotar(id, chat, msgSoTexto)
		}
		return
	}
	if cortado {
		texto += " [áudio longo: ouvido só até os 5 minutos]"
	}
	m.Texto = texto
	iniciar, avisarLimite := false, false
	if podeResponder {
		podeResponder, iniciar, avisarLimite = a.entrarNaVez(id, cfg, agora)
		m.Pendente = podeResponder
	}
	a.salvarLeads()
	a.mu.Unlock()

	log.Printf("OUVIDO: áudio de %s transcrito (%d letras, responder=%v)", fimDoNumero(id), len([]rune(texto)), podeResponder)
	if avisarLimite {
		go a.enviarEAnotar(id, chat, msgUmaPorVez)
	}
	if podeResponder {
		go a.marcarLida(cli, info)
	}
	if iniciar {
		go a.trabalhar(id)
	}
}

// anotarRespostaDoCelular guarda na conversa o que a pessoa do negocio escreveu pelo
// proprio celular e passa a conversa para ela: o assistente nao fala por cima.
func (a *App) anotarRespostaDoCelular(cli *whatsmeow.Client, info types.MessageInfo, chat types.JID, msg Msg, midia *infoMidia) {
	alt := info.RecipientAlt.ToNonAD()
	tel := ""
	switch {
	case chat.Server == types.DefaultUserServer:
		tel = chat.User
	case !alt.IsEmpty() && alt.Server == types.DefaultUserServer:
		tel = alt.User
	default:
		tel = telefoneDoLID(cli, chat)
	}

	a.mu.Lock()
	l := a.leads[chat.String()]
	if l == nil && !alt.IsEmpty() {
		l = a.leads[alt.String()]
	}
	if l == nil && tel != "" {
		for _, candidato := range a.leads {
			if mesmoNumero(candidato.Telefone, tel) {
				l = candidato
				break
			}
		}
	}
	if l == nil {
		a.mu.Unlock()
		return // conversa que nao e de um lead: nao e da nossa conta
	}
	msg.De = "humano"
	msg.Hora = time.Now()
	if !info.Timestamp.IsZero() && msg.Hora.Sub(info.Timestamp) < idadeMaximaMsg {
		msg.Hora = info.Timestamp
	}
	l.anexar(msg)
	if l.Status != statusAtendimento {
		l.Status = statusAtendimento
		l.Atendente = ""
		l.soltarPendentes()
		l.evento("assumiu", "A equipe respondeu pelo celular e assumiu a conversa")
		log.Printf("MENSAGEM: a pessoa respondeu pelo celular, o assistente parou nessa conversa (%s)", fimDoNumero(l.Telefone))
	}
	leadID := l.ID
	a.salvarLeads()
	a.mu.Unlock()
	if midia != nil {
		go a.baixarMidia(cli, leadID, info.ID, midia)
	}
}

func (a *App) marcarLida(cli *whatsmeow.Client, info types.MessageInfo) {
	ctx, cancelar := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancelar()
	if err := cli.MarkRead(ctx, []types.MessageID{info.ID}, time.Now(), info.Chat, info.Sender); err != nil {
		log.Printf("WHATSAPP: não marcou como lida: %v", err)
	}
}

func (a *App) marcarDigitando(chat types.JID, sim bool) {
	cli := a.cliente()
	if cli == nil || !cli.IsConnected() || !cli.IsLoggedIn() {
		return
	}
	estado := types.ChatPresencePaused
	if sim {
		estado = types.ChatPresenceComposing
	}
	ctx, cancelar := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancelar()
	_ = cli.SendChatPresence(ctx, chat, estado, types.ChatPresenceMediaText)
}

// presenca fica "online" so enquanto responde. Assim o celular do negocio continua
// recebendo as notificacoes no resto do tempo.
func (a *App) presenca(delta int) {
	a.mu.Lock()
	antes := a.digitam
	a.digitam += delta
	depois := a.digitam
	cli := a.cli
	a.mu.Unlock()
	if cli == nil || !cli.IsConnected() || !cli.IsLoggedIn() {
		return
	}
	ctx, cancelar := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancelar()
	if antes == 0 && depois > 0 {
		_ = cli.SendPresence(ctx, types.PresenceAvailable)
	} else if antes > 0 && depois == 0 {
		_ = cli.SendPresence(ctx, types.PresenceUnavailable)
	}
}

func (a *App) enviarTexto(chat types.JID, texto string) error {
	cli := a.cliente()
	if cli == nil || !cli.IsConnected() || !cli.IsLoggedIn() {
		return errors.New("WhatsApp desconectado")
	}
	ctx, cancelar := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancelar()
	_, err := cli.SendMessage(ctx, chat, &waE2E.Message{Conversation: proto.String(texto)})
	return err
}

// enviarEAnotar manda um aviso curto e guarda no historico da conversa.
func (a *App) enviarEAnotar(id string, chat types.JID, texto string) {
	if err := a.enviarTexto(chat, texto); err != nil {
		log.Printf("WHATSAPP: não enviou o aviso: %v", err)
		return
	}
	a.mu.Lock()
	if l := a.leads[id]; l != nil {
		l.anexar(Msg{De: "atendente", Texto: texto, Hora: time.Now()})
		a.salvarLeads()
	}
	a.mu.Unlock()
}

// consultaDaConversa junta o que o cliente acabou de dizer com o fim da conversa,
// para a busca nos materiais entender respostas curtas como "esse mesmo".
func consultaDaConversa(anteriores []Msg, novas []string) string {
	partes := append([]string(nil), novas...)
	clientes, atendente := 0, false
	for i := len(anteriores) - 1; i >= 0 && (clientes < 2 || !atendente); i-- {
		m := anteriores[i]
		switch {
		case m.De == "cliente" && clientes < 2:
			clientes++
			partes = append(partes, m.Texto)
		case m.De != "cliente" && !atendente:
			atendente = true
			partes = append(partes, m.Texto)
		}
	}
	return strings.Join(partes, " ")
}

// trabalhar responde uma conversa: espera alguns segundos com "digitando", junta as
// mensagens seguidas em UMA resposta e chama o cerebro (no maximo 3 ao mesmo tempo).
func (a *App) trabalhar(id string) {
	soltar := func() {
		a.mu.Lock()
		if c := a.contatos[id]; c != nil {
			c.trabalhando = false
		}
		if l := a.leads[id]; l != nil {
			l.soltarPendentes()
		}
		a.mu.Unlock()
	}
	chat, err := types.ParseJID(id)
	if err != nil {
		soltar()
		return
	}
	a.presenca(1)
	defer a.presenca(-1)

	for {
		a.marcarDigitando(chat, true)
		a.mu.Lock()
		min, max := a.cfg.AtrasoMin, a.cfg.AtrasoMax
		a.mu.Unlock()
		time.Sleep(time.Duration(min*1000+rand.Intn((max-min)*1000+1)) * time.Millisecond)

		a.mu.Lock()
		l := a.leads[id]
		if l == nil || !a.cfg.Ligado || !a.cfg.Salvo || l.comHumano() {
			a.mu.Unlock()
			soltar()
			a.marcarDigitando(chat, false)
			return
		}
		var anteriores []Msg
		var novas []string
		for i := range l.Historico {
			if l.Historico[i].Pendente {
				novas = append(novas, textoParaIA(l.Historico[i]))
				l.Historico[i].Pendente = false
			} else {
				anteriores = append(anteriores, l.Historico[i])
			}
		}
		if len(novas) == 0 {
			a.mu.Unlock()
			soltar()
			a.marcarDigitando(chat, false)
			return
		}
		cfg := a.cfg.clonar()
		entrada := montarEntrada(anteriores, novas, l, a.etiquetasEmUso())
		a.mu.Unlock()

		// Tudo relido do disco a cada mensagem: textos, materiais e agenda.
		sistema, _ := a.montarSistema(cfg, consultaDaConversa(anteriores, novas), id)

		a.vagas <- struct{}{}
		parar := make(chan struct{})
		go func() {
			t := time.NewTicker(8 * time.Second)
			defer t.Stop()
			for {
				select {
				case <-parar:
					return
				case <-t.C:
					a.marcarDigitando(chat, true)
				}
			}
		}()
		saida, modo, erroCerebro := a.cerebro.Responder(modeloDe(cfg), sistema, entrada, strings.Join(novas, " "))
		close(parar)
		<-a.vagas

		if erroCerebro != nil {
			log.Printf("CEREBRO: sem resposta, passando para uma pessoa: %v", erroCerebro)
			saida = Saida{
				Resposta:   respostaFalha,
				Transbordo: true,
				Motivo:     "o assistente não conseguiu responder (falha técnica)",
				Lead:       LeadIA{Classificacao: "indefinido"},
			}
		}
		a.marcarDigitando(chat, false)

		// Se pausaram o atendente ou uma pessoa assumiu durante a espera, nao envia.
		// A agenda e conferida antes de enviar: se o horario nao der, a resposta muda.
		a.mu.Lock()
		l = a.leads[id]
		if l == nil || !a.cfg.Ligado || l.comHumano() {
			a.mu.Unlock()
			soltar()
			return
		}
		agenda := a.aplicarAgendamento(l, saida.Agendamento, limparCampo(saida.Lead.Nome), time.Now())
		if agenda.Marcado != nil {
			l.evento("agendou", "Horário marcado pelo assistente: "+agenda.Marcado.Servico+", "+dataPorExtenso(agenda.Marcado.Inicio))
		}
		if agenda.Cancelado != nil {
			l.evento("cancelou", "Horário cancelado pelo assistente: "+agenda.Cancelado.Servico+", "+dataPorExtenso(agenda.Cancelado.Inicio))
		}
		if agenda.Remarcado != nil {
			l.evento("remarcou", "Horário remarcado pelo assistente: "+agenda.Remarcado.Servico+", de "+dataPorExtenso(agenda.Antes)+" para "+dataPorExtenso(agenda.Remarcado.Inicio))
		}
		a.mu.Unlock()
		if agenda.Resposta != "" {
			saida.Resposta = agenda.Resposta
		}
		if agenda.Transbordo {
			saida.Transbordo = true
			saida.Motivo = ouPadrao(saida.Motivo, "sem horário livre na agenda")
		}

		erroEnvio := a.enviarTexto(chat, saida.Resposta)
		if erroEnvio != nil {
			time.Sleep(3 * time.Second)
			erroEnvio = a.enviarTexto(chat, saida.Resposta)
		}
		if erroEnvio != nil {
			log.Printf("WHATSAPP: não enviou a resposta para %s: %v", fimDoNumero(id), erroEnvio)
		}

		a.mu.Lock()
		l = a.leads[id]
		if l == nil {
			a.mu.Unlock()
			soltar()
			return
		}
		aplicarSaida(l, saida)
		if postas, tiradas := l.etiquetasDaIA(saida.Lead.Etiquetas, a.etiquetasEmUso()); len(postas)+len(tiradas) > 0 {
			l.evento("etiqueta", fraseDeEtiquetas("a IA", postas, tiradas))
		}
		quente := l.Classificacao == "quente"
		transbordo := saida.Transbordo
		// Lead quente sempre chama uma pessoa. Com a agenda ligada, quem decide e o
		// assistente: quem so quer marcar um horario nao precisa de uma pessoa.
		if quente && !l.Retomado && !a.cfg.Agenda.Ativa {
			transbordo = true
		}
		if erroEnvio == nil {
			l.anexar(Msg{De: "atendente", Texto: saida.Resposta, Hora: time.Now()})
		}
		mais := false
		if transbordo {
			padrao := "pediu atendimento humano"
			if quente {
				padrao = "lead quente"
			}
			a.assumir(l, ouPadrao(limparCampo(saida.Motivo), padrao))
			l.Atendente = ""
			l.evento("transbordo", "Transferido para a equipe: "+l.Motivo)
		} else {
			if l.Classificacao != "indefinido" {
				l.Status = statusQualificado
			} else {
				l.Status = statusQualificando
			}
			for i := range l.Historico {
				if l.Historico[i].Pendente {
					mais = true
					break
				}
			}
		}
		if !mais {
			if c := a.contatos[id]; c != nil {
				c.trabalhando = false
			}
		}
		copia := *l
		numeros := append([]string(nil), a.cfg.NumerosHumanos...)
		avisarAgenda := a.cfg.Agenda.AvisarHumano
		a.salvarLeads()
		a.mu.Unlock()

		log.Printf("RESPOSTA: para %s (cérebro=%s, classe=%s, pontos=%d, transbordo=%v)",
			fimDoNumero(copia.Telefone), modo, copia.Classificacao, copia.Pontuacao, transbordo)

		if agenda.Marcado != nil {
			log.Printf("AGENDA: horário marcado pelo assistente para %s", dataBR(agenda.Marcado.Inicio))
			if avisarAgenda {
				go a.avisarHumanos(numeros, avisoAgendamento(agenda.Marcado, false), "agendamento")
			}
		}
		if agenda.Cancelado != nil {
			log.Printf("AGENDA: horário cancelado pelo assistente")
			if avisarAgenda {
				go a.avisarHumanos(numeros, avisoAgendamento(agenda.Cancelado, true), "cancelamento")
			}
		}
		if agenda.Remarcado != nil {
			log.Printf("AGENDA: horário remarcado pelo assistente para %s", dataBR(agenda.Remarcado.Inicio))
			if avisarAgenda {
				go a.avisarHumanos(numeros, avisoRemarcacao(agenda.Remarcado, agenda.Antes), "remarcação")
			}
		}
		if transbordo {
			go a.avisarHumano(copia, numeros)
			return
		}
		if !mais {
			return
		}
	}
}

// avisarNumeroHumano manda um aviso para o numero que o dono cadastrou.
// Sao as unicas mensagens que o programa inicia sozinho.
func (a *App) avisarNumeroHumano(numero, texto, titulo string) {
	num := soDigitos(numero)
	if num == "" {
		return
	}
	cli := a.cliente()
	if cli == nil || !cli.IsConnected() || !cli.IsLoggedIn() {
		log.Printf("AVISO HUMANO: WhatsApp desconectado, não deu para avisar")
		return
	}
	destino := types.NewJID(num, types.DefaultUserServer)
	ctx, cancelar := context.WithTimeout(context.Background(), 20*time.Second)
	if resp, err := cli.IsOnWhatsApp(ctx, []string{"+" + num}); err == nil {
		// No Brasil o WhatsApp pode guardar o celular sem o nono digito: usa o endereco certo.
		for _, r := range resp {
			if !r.IsIn {
				continue
			}
			if r.JID.Server == types.DefaultUserServer && r.JID.User != "" {
				destino = r.JID.ToNonAD()
			} else if r.PhoneNumber.Server == types.DefaultUserServer && r.PhoneNumber.User != "" {
				destino = r.PhoneNumber.ToNonAD()
			}
			break
		}
	}
	cancelar()

	if err := a.enviarTexto(destino, texto); err != nil {
		log.Printf("AVISO HUMANO: não enviou: %v", err)
		return
	}
	log.Printf("AVISO HUMANO: enviado (%s)", titulo)
}

// avisarHumanos manda o mesmo aviso para todos os numeros cadastrados.
func (a *App) avisarHumanos(numeros []string, texto, titulo string) {
	for _, n := range numeros {
		a.avisarNumeroHumano(n, texto, titulo)
	}
}

// avisarHumano manda o resumo do lead para os numeros que recebem os avisos.
func (a *App) avisarHumano(l Lead, numeros []string) {
	titulo := "PEDIU HUMANO"
	if l.Classificacao == "quente" {
		titulo = "LEAD QUENTE"
	}
	nome := ouPadrao(l.Nome, ouPadrao(l.NomeWhats, "sem nome ainda"))
	var b strings.Builder
	b.WriteString(titulo + ": " + nome + "\n")
	b.WriteString("Telefone: " + telefoneBonito(l.Telefone) + "\n")
	b.WriteString("Resumo: " + ouPadrao(l.Resumo, "ainda sem resumo") + "\n")
	b.WriteString("O que precisa: " + ouPadrao(l.Necessidade, "ainda não disse") + "\n")
	b.WriteString("Prazo: " + ouPadrao(l.Prazo, "ainda não disse") + "\n")
	if l.Motivo != "" && l.Classificacao != "quente" {
		b.WriteString("Motivo: " + l.Motivo + "\n")
	}
	b.WriteString("Responda direto pelo WhatsApp do negócio.\n")
	b.WriteString("Para devolver ao assistente depois, mande: /retomar " + soDigitos(l.Telefone))
	a.avisarHumanos(numeros, b.String(), titulo)
}

// comandoHumano atende /lista e /retomar TELEFONE. So o numero humano chega aqui.
func (a *App) comandoHumano(chat types.JID, texto string) {
	campos := strings.Fields(texto)
	if len(campos) == 0 {
		return
	}
	resposta := "Comandos: /lista mostra quem aguarda uma pessoa. /retomar TELEFONE devolve o lead ao assistente."

	switch strings.ToLower(campos[0]) {
	case "/lista":
		a.mu.Lock()
		var linhas []string
		for _, l := range a.leadsOrdenados() {
			if !l.comHumano() {
				continue
			}
			nome := ouPadrao(l.Nome, ouPadrao(l.NomeWhats, "sem nome"))
			linhas = append(linhas, fmt.Sprintf("%d) %s, %s, %s", len(linhas)+1, nome, soDigitos(l.Telefone), ouPadrao(l.Classificacao, "indefinido")))
		}
		a.mu.Unlock()
		if len(linhas) == 0 {
			resposta = "Ninguém aguardando uma pessoa no momento."
		} else {
			resposta = "Aguardando uma pessoa:\n" + strings.Join(linhas, "\n") + "\nPara devolver ao assistente: /retomar TELEFONE"
		}

	case "/retomar":
		alvo := chaveNumero(strings.Join(campos[1:], ""))
		if len(alvo) < 8 {
			resposta = "Mande assim: /retomar TELEFONE (com DDI e DDD, só números). Use /lista para ver os telefones."
			break
		}
		a.mu.Lock()
		var achados []*Lead
		for _, l := range a.leads {
			if l.comHumano() && strings.HasSuffix(chaveNumero(l.Telefone), alvo) {
				achados = append(achados, l)
			}
		}
		switch len(achados) {
		case 0:
			resposta = "Não achei esse telefone entre os que aguardam uma pessoa. Mande /lista para conferir."
		case 1:
			a.retomar(achados[0])
			achados[0].evento("devolveu", "A equipe devolveu a conversa ao assistente (comando pelo WhatsApp)")
			a.salvarLeads()
			nome := ouPadrao(achados[0].Nome, ouPadrao(achados[0].NomeWhats, "O lead"))
			resposta = "Pronto. " + nome + " voltou para o assistente."
		default:
			resposta = "Achei mais de um telefone com esse final. Mande o número completo, com DDI e DDD."
		}
		a.mu.Unlock()
	}

	if err := a.enviarTexto(chat, resposta); err != nil {
		log.Printf("COMANDO: não respondeu ao número humano: %v", err)
	}
}
