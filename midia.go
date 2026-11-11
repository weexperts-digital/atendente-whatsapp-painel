package main

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/types"
	"google.golang.org/protobuf/proto"
)

// Midias (imagem, audio, video, documento) e fotos de perfil.
//
// O que chega do cliente e baixado SO dos servidores do WhatsApp (a biblioteca usa o
// caminho interno da midia, nunca um endereco escrito na mensagem), guardado em
// dados/midia e mostrado no painel. Nada e aberto nem executado pelo programa, e a
// IA continua lendo so texto.

const maxMidia = 25 << 20

var (
	pastaMidia = filepath.Join(pastaDados, "midia")
	pastaFotos = filepath.Join(pastaDados, "fotos")
	reMidia    = regexp.MustCompile(`^f[0-9a-f]{10}\.[a-z0-9]{1,5}$`)
)

// O que o painel pode abrir direto no navegador. O resto e sempre baixado.
var tiposInline = map[string]string{
	".jpg": "image/jpeg", ".png": "image/png", ".webp": "image/webp", ".gif": "image/gif",
	".ogg": "audio/ogg", ".mp3": "audio/mpeg", ".m4a": "audio/mp4", ".aac": "audio/aac", ".wav": "audio/wav",
	".mp4": "video/mp4", ".webm": "video/webm", ".3gp": "video/3gpp", ".mov": "video/quicktime",
	".pdf": "application/pdf",
}

var extPorMime = map[string]string{
	"image/jpeg": ".jpg", "image/png": ".png", "image/webp": ".webp", "image/gif": ".gif",
	"audio/ogg": ".ogg", "application/ogg": ".ogg", "audio/mpeg": ".mp3", "audio/mp4": ".m4a", "audio/x-m4a": ".m4a",
	"audio/aac": ".aac", "audio/wav": ".wav", "audio/wave": ".wav", "audio/x-wav": ".wav", "audio/webm": ".webm",
	"video/mp4": ".mp4", "video/webm": ".webm", "video/3gpp": ".3gp", "video/quicktime": ".mov",
	"application/pdf": ".pdf",
}

func mimeLimpo(m string) string {
	if i := strings.IndexByte(m, ';'); i >= 0 {
		m = m[:i]
	}
	return strings.ToLower(strings.TrimSpace(m))
}

// extensaoDe escolhe a extensao do arquivo guardado. So letras e numeros, para o
// nome nunca sair da pasta de midias.
func extensaoDe(mime, nome string) string {
	if e, ok := extPorMime[mimeLimpo(mime)]; ok {
		return e
	}
	e := strings.ToLower(strings.TrimPrefix(filepath.Ext(nome), "."))
	if len(e) >= 1 && len(e) <= 5 {
		limpa := true
		for _, r := range e {
			if !(r >= 'a' && r <= 'z') && !(r >= '0' && r <= '9') {
				limpa = false
			}
		}
		// html e svg podem carregar script: guardamos como arquivo generico.
		if limpa && e != "html" && e != "htm" && e != "svg" && e != "xml" && e != "js" {
			return "." + e
		}
	}
	return ".bin"
}

// caminhoMidia confere o nome e devolve o caminho dentro de dados/midia.
func caminhoMidia(id string) (string, bool) {
	if !reMidia.MatchString(id) {
		return "", false
	}
	return filepath.Join(pastaMidia, id), true
}

type infoMidia struct {
	Tipo    string // imagem, audio, video, documento ou outro (localizacao, contato)
	Nome    string
	Mime    string
	Tamanho int
	Legenda string
	Baixar  whatsmeow.DownloadableMessage
}

// rotulo e o texto que fica na conversa quando a midia vem sem legenda.
func (m *infoMidia) rotulo() string {
	switch m.Tipo {
	case "imagem":
		return "[imagem]"
	case "audio":
		return "[áudio]"
	case "video":
		return "[vídeo]"
	case "documento":
		return "[arquivo] " + m.Nome
	}
	return "[localização ou contato]"
}

// midiaDe diz se a mensagem traz uma midia e de que tipo.
func midiaDe(m *waE2E.Message) *infoMidia {
	switch {
	case m.GetImageMessage() != nil:
		x := m.GetImageMessage()
		return &infoMidia{Tipo: "imagem", Mime: x.GetMimetype(), Tamanho: int(x.GetFileLength()), Legenda: x.GetCaption(), Baixar: x}
	case m.GetStickerMessage() != nil:
		x := m.GetStickerMessage()
		return &infoMidia{Tipo: "imagem", Nome: "figurinha", Mime: x.GetMimetype(), Tamanho: int(x.GetFileLength()), Baixar: x}
	case m.GetAudioMessage() != nil:
		x := m.GetAudioMessage()
		return &infoMidia{Tipo: "audio", Mime: x.GetMimetype(), Tamanho: int(x.GetFileLength()), Baixar: x}
	case m.GetVideoMessage() != nil:
		x := m.GetVideoMessage()
		return &infoMidia{Tipo: "video", Mime: x.GetMimetype(), Tamanho: int(x.GetFileLength()), Legenda: x.GetCaption(), Baixar: x}
	case m.GetPtvMessage() != nil:
		x := m.GetPtvMessage()
		return &infoMidia{Tipo: "video", Mime: x.GetMimetype(), Tamanho: int(x.GetFileLength()), Baixar: x}
	case m.GetDocumentMessage() != nil:
		x := m.GetDocumentMessage()
		nome := ouPadrao(x.GetFileName(), ouPadrao(x.GetTitle(), "arquivo"))
		return &infoMidia{Tipo: "documento", Nome: cortar(nome, 160), Mime: x.GetMimetype(), Tamanho: int(x.GetFileLength()), Legenda: x.GetCaption(), Baixar: x}
	case m.GetLocationMessage() != nil, m.GetLiveLocationMessage() != nil, m.GetContactMessage() != nil, m.GetContactsArrayMessage() != nil:
		return &infoMidia{Tipo: "outro"}
	}
	return nil
}

// baixarMidia busca o arquivo nos servidores do WhatsApp e liga na mensagem ja guardada.
func (a *App) baixarMidia(cli *whatsmeow.Client, leadID string, msgID types.MessageID, m *infoMidia) {
	if m == nil || m.Baixar == nil || m.Baixar.GetDirectPath() == "" || m.Tamanho > maxMidia {
		return
	}
	a.baixando <- struct{}{}
	defer func() { <-a.baixando }()

	ctx, cancelar := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancelar()
	dados, err := cli.Download(ctx, m.Baixar)
	if err != nil || len(dados) == 0 || len(dados) > maxMidia {
		log.Printf("MIDIA: não baixei um arquivo de %s: %v", fimDoNumero(leadID), err)
		return
	}
	nome := novoID("f") + extensaoDe(m.Mime, m.Nome)
	if err := gravarSeguro(filepath.Join(pastaMidia, nome), dados, 0o600); err != nil {
		log.Printf("MIDIA: não guardei o arquivo: %v", err)
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	l := a.leads[leadID]
	if l == nil {
		_ = os.Remove(filepath.Join(pastaMidia, nome))
		return
	}
	for i := len(l.Historico) - 1; i >= 0; i-- {
		if l.Historico[i].ID == string(msgID) {
			l.Historico[i].Midia = nome
			l.Historico[i].Tamanho = len(dados)
			a.salvarLeads()
			return
		}
	}
	_ = os.Remove(filepath.Join(pastaMidia, nome))
}

// enviarMidia manda um arquivo escolhido pela pessoa no painel e devolve a mensagem
// para guardar no historico.
func (a *App) enviarMidia(chat types.JID, dados []byte, nome, mime, legenda string, voz bool) (Msg, error) {
	cli := a.cliente()
	if cli == nil || !cli.IsConnected() || !cli.IsLoggedIn() {
		return Msg{}, errors.New("WhatsApp desconectado")
	}
	mime = mimeLimpo(mime)
	if mime == "" || mime == "application/octet-stream" {
		mime = mimeLimpo(http.DetectContentType(dados))
	}
	nome = cortar(strings.TrimSpace(filepath.Base(strings.ReplaceAll(nome, "\\", "/"))), 160)
	if nome == "" || nome == "." {
		nome = "arquivo"
	}
	legenda = cortar(strings.TrimSpace(legenda), 1000)

	tipo, classe := "documento", whatsmeow.MediaDocument
	switch {
	case mime == "image/jpeg" || mime == "image/png":
		tipo, classe = "imagem", whatsmeow.MediaImage
	case mime == "video/mp4" || mime == "video/3gpp":
		tipo, classe = "video", whatsmeow.MediaVideo
	case strings.HasPrefix(mime, "audio/") && mime != "audio/webm", mime == "application/ogg":
		tipo, classe = "audio", whatsmeow.MediaAudio
	}

	ctx, cancelar := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancelar()
	up, err := cli.Upload(ctx, dados, classe)
	if err != nil {
		return Msg{}, err
	}
	var msg waE2E.Message
	switch tipo {
	case "imagem":
		msg.ImageMessage = &waE2E.ImageMessage{
			Caption: proto.String(legenda), Mimetype: proto.String(mime),
			URL: proto.String(up.URL), DirectPath: proto.String(up.DirectPath), MediaKey: up.MediaKey,
			FileEncSHA256: up.FileEncSHA256, FileSHA256: up.FileSHA256, FileLength: proto.Uint64(up.FileLength),
		}
	case "video":
		msg.VideoMessage = &waE2E.VideoMessage{
			Caption: proto.String(legenda), Mimetype: proto.String(mime),
			URL: proto.String(up.URL), DirectPath: proto.String(up.DirectPath), MediaKey: up.MediaKey,
			FileEncSHA256: up.FileEncSHA256, FileSHA256: up.FileSHA256, FileLength: proto.Uint64(up.FileLength),
		}
	case "audio":
		tipoZap := mime
		ogg := mime == "audio/ogg" || mime == "application/ogg"
		if ogg {
			tipoZap = "audio/ogg; codecs=opus"
		}
		msg.AudioMessage = &waE2E.AudioMessage{
			Mimetype: proto.String(tipoZap), PTT: proto.Bool(voz && ogg),
			URL: proto.String(up.URL), DirectPath: proto.String(up.DirectPath), MediaKey: up.MediaKey,
			FileEncSHA256: up.FileEncSHA256, FileSHA256: up.FileSHA256, FileLength: proto.Uint64(up.FileLength),
		}
		legenda = "" // audio nao leva legenda
	default:
		msg.DocumentMessage = &waE2E.DocumentMessage{
			FileName: proto.String(nome), Title: proto.String(nome), Caption: proto.String(legenda), Mimetype: proto.String(ouPadrao(mime, "application/octet-stream")),
			URL: proto.String(up.URL), DirectPath: proto.String(up.DirectPath), MediaKey: up.MediaKey,
			FileEncSHA256: up.FileEncSHA256, FileSHA256: up.FileSHA256, FileLength: proto.Uint64(up.FileLength),
		}
	}
	if _, err := cli.SendMessage(ctx, chat, &msg); err != nil {
		return Msg{}, err
	}

	// Guarda uma copia para o painel mostrar o que foi enviado.
	arquivo := novoID("f") + extensaoDe(mime, nome)
	if err := gravarSeguro(filepath.Join(pastaMidia, arquivo), dados, 0o600); err != nil {
		arquivo = ""
	}
	return Msg{De: "humano", Texto: legenda, Hora: time.Now(), Tipo: tipo, Midia: arquivo, Nome: nome, Mime: mime, Tamanho: len(dados)}, nil
}

// apagarMidias remove os arquivos de uma conversa apagada.
func apagarMidias(l *Lead) {
	for _, m := range l.Historico {
		if p, ok := caminhoMidia(m.Midia); ok {
			_ = os.Remove(p)
		}
	}
	_ = os.Remove(arquivoFoto(l.ID))
}

// ---------- foto de perfil ----------

func arquivoFoto(leadID string) string {
	soma := sha1.Sum([]byte(leadID))
	return filepath.Join(pastaFotos, hex.EncodeToString(soma[:8])+".jpg")
}

// atualizarFoto busca a foto de perfil do contato, no maximo uma vez por dia.
func (a *App) atualizarFoto(cli *whatsmeow.Client, leadID string) {
	a.mu.Lock()
	l := a.leads[leadID]
	if l == nil || time.Since(l.FotoEm) < 24*time.Hour {
		a.mu.Unlock()
		return
	}
	l.FotoEm = time.Now() // marca ja, para nao buscar duas vezes ao mesmo tempo
	atual := l.FotoID
	a.mu.Unlock()

	jid, err := types.ParseJID(leadID)
	if err != nil {
		return
	}
	ctx, cancelar := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancelar()
	info, err := cli.GetProfilePictureInfo(ctx, jid, &whatsmeow.GetProfilePictureParams{Preview: true, ExistingID: atual})
	fotoID := atual
	switch {
	case errors.Is(err, whatsmeow.ErrProfilePictureNotSet), errors.Is(err, whatsmeow.ErrProfilePictureUnauthorized):
		fotoID = ""
		_ = os.Remove(arquivoFoto(leadID))
	case err != nil:
		return
	case info != nil && info.URL != "":
		dados, err := baixarFoto(ctx, info.URL)
		if err != nil {
			log.Printf("FOTO: não baixei a foto de %s: %v", fimDoNumero(leadID), err)
			return
		}
		if err := gravarSeguro(arquivoFoto(leadID), dados, 0o600); err != nil {
			return
		}
		fotoID = ouPadrao(info.ID, "1")
	}
	a.mu.Lock()
	if l := a.leads[leadID]; l != nil && l.FotoID != fotoID {
		l.FotoID = fotoID
		a.salvarLeads()
	}
	a.mu.Unlock()
}

// atualizarFotos confere, sem pressa, a foto das conversas mais recentes.
func (a *App) atualizarFotos(cli *whatsmeow.Client) {
	a.mu.Lock()
	var ids []string
	for _, l := range a.leadsOrdenados() {
		if len(ids) < 60 && time.Since(l.FotoEm) > 24*time.Hour {
			ids = append(ids, l.ID)
		}
	}
	a.mu.Unlock()
	for _, id := range ids {
		if !cli.IsConnected() || !cli.IsLoggedIn() {
			return
		}
		a.atualizarFoto(cli, id)
		time.Sleep(400 * time.Millisecond)
	}
}

// baixarFoto so aceita endereco https dos servidores do WhatsApp e arquivo de imagem pequeno.
func baixarFoto(ctx context.Context, endereco string) ([]byte, error) {
	u, err := url.Parse(endereco)
	if err != nil || u.Scheme != "https" || !(strings.HasSuffix(u.Hostname(), ".whatsapp.net") || strings.HasSuffix(u.Hostname(), ".whatsapp.com")) {
		return nil, errors.New("endereço de foto fora do WhatsApp")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endereco, nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, errors.New("o servidor não entregou a foto")
	}
	dados, err := io.ReadAll(io.LimitReader(resp.Body, 3<<20))
	if err != nil {
		return nil, err
	}
	if tipo := http.DetectContentType(dados); tipo != "image/jpeg" && tipo != "image/png" && tipo != "image/webp" {
		return nil, errors.New("o arquivo recebido não é uma imagem")
	}
	return dados, nil
}

// ---------- etiquetas ----------

// limparEtiquetas tira repetidas e vazias: ate 12 por conversa, 30 letras cada.
func limparEtiquetas(lista []string) []string {
	saida := make([]string, 0, len(lista))
	vistas := map[string]bool{}
	for _, e := range lista {
		e = cortar(umaLinha(e), 30)
		chave := chaveTexto(e)
		if e == "" || vistas[chave] || len(saida) >= 12 {
			continue
		}
		vistas[chave] = true
		saida = append(saida, e)
	}
	return saida
}

// etiquetasEmUso lista todas as etiquetas das conversas. Chame com a.mu travado.
func (a *App) etiquetasEmUso() []string {
	vistas := map[string]bool{}
	lista := []string{}
	for _, l := range a.leads {
		for _, e := range l.Etiquetas {
			if !vistas[e] {
				vistas[e] = true
				lista = append(lista, e)
			}
		}
	}
	sort.Slice(lista, func(i, j int) bool { return chaveTexto(lista[i]) < chaveTexto(lista[j]) })
	return lista
}

// ---------- relatorio ----------

type contagem struct {
	Nome  string `json:"nome"`
	Total int    `json:"total"`
}

type diaRelatorio struct {
	Data         string `json:"data"`
	Rotulo       string `json:"rotulo"`
	Novas        int    `json:"novas"`
	Mensagens    int    `json:"mensagens"`
	Transbordos  int    `json:"transbordos"`
	Agendamentos int    `json:"agendamentos"`
}

// montarRelatorio conta o que aconteceu nos ultimos dias (0 = tudo).
func montarRelatorio(leads []*Lead, agenda []*Agendamento, dias int, agora time.Time) map[string]any {
	hoje := time.Date(agora.Year(), agora.Month(), agora.Day(), 0, 0, 0, 0, agora.Location())
	var inicio time.Time // zero = sem limite
	if dias > 0 {
		inicio = hoje.AddDate(0, 0, -(dias - 1))
	}
	dentro := func(t time.Time) bool { return !t.IsZero() && (inicio.IsZero() || !t.Before(inicio)) }

	// Um item por dia, do mais antigo para o mais novo.
	nDias := dias
	if nDias <= 1 {
		nDias = 14
	}
	porDia := make([]diaRelatorio, nDias)
	indice := map[string]int{}
	for i := 0; i < nDias; i++ {
		d := hoje.AddDate(0, 0, -(nDias - 1 - i))
		porDia[i] = diaRelatorio{Data: d.Format("2006-01-02"), Rotulo: diaCurto(d)}
		indice[porDia[i].Data] = i
	}
	noDia := func(t time.Time) *diaRelatorio {
		if i, ok := indice[t.In(agora.Location()).Format("2006-01-02")]; ok {
			return &porDia[i]
		}
		return nil
	}

	periodo := map[string]int{"conversas": 0, "novas": 0, "mensagens_clientes": 0, "respostas_assistente": 0, "respostas_equipe": 0, "transbordos": 0, "agendamentos": 0, "cancelamentos": 0}
	atual := map[string]int{"total": len(leads), "com_assistente": 0, "aguardando": 0, "em_atendimento": 0, "quentes": 0, "mornos": 0, "frios": 0, "indefinidos": 0}
	etiquetas := map[string]int{}
	equipe := map[string]int{}

	for _, l := range leads {
		switch l.Status {
		case statusHumano:
			atual["aguardando"]++
		case statusAtendimento:
			atual["em_atendimento"]++
		default:
			atual["com_assistente"]++
		}
		switch l.Classificacao {
		case "quente":
			atual["quentes"]++
		case "morno":
			atual["mornos"]++
		case "frio":
			atual["frios"]++
		default:
			atual["indefinidos"]++
		}
		for _, e := range l.Etiquetas {
			etiquetas[e]++
		}
		if dentro(l.CriadoEm) {
			periodo["novas"]++
		}
		if d := noDia(l.CriadoEm); d != nil {
			d.Novas++
		}
		ativa := false
		for _, m := range l.Historico {
			noPeriodo := dentro(m.Hora)
			d := noDia(m.Hora)
			switch {
			case m.De == "sistema":
				if m.Evento == "transbordo" {
					if noPeriodo {
						periodo["transbordos"]++
					}
					if d != nil {
						d.Transbordos++
					}
				}
				continue
			case m.De == "cliente":
				if noPeriodo {
					periodo["mensagens_clientes"]++
				}
			case m.De == "atendente":
				if noPeriodo {
					periodo["respostas_assistente"]++
				}
			case m.De == "humano":
				if noPeriodo {
					periodo["respostas_equipe"]++
					equipe[ouPadrao(m.Autor, "Equipe (sem nome)")]++
				}
			}
			if noPeriodo {
				ativa = true
			}
			if d != nil {
				d.Mensagens++
			}
		}
		if ativa {
			periodo["conversas"]++
		}
	}
	for _, ag := range agenda {
		if !dentro(ag.CriadoEm) {
			continue
		}
		if ag.Status == "cancelado" {
			periodo["cancelamentos"]++
			continue
		}
		periodo["agendamentos"]++
		if d := noDia(ag.CriadoEm); d != nil {
			d.Agendamentos++
		}
	}

	ordenar := func(m map[string]int) []contagem {
		lista := make([]contagem, 0, len(m))
		for nome, total := range m {
			lista = append(lista, contagem{nome, total})
		}
		sort.Slice(lista, func(i, j int) bool {
			if lista[i].Total != lista[j].Total {
				return lista[i].Total > lista[j].Total
			}
			return lista[i].Nome < lista[j].Nome
		})
		return lista
	}
	atendentes := []map[string]any{}
	for _, c := range ordenar(equipe) {
		atendentes = append(atendentes, map[string]any{"nome": c.Nome, "respostas": c.Total})
	}
	return map[string]any{
		"dias": dias, "periodo": periodo, "agora": atual,
		"por_etiqueta": ordenar(etiquetas), "por_dia": porDia, "atendentes": atendentes,
	}
}
