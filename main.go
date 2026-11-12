package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"syscall"
	"time"
)

func main() {
	simular := flag.String("simular", "", "passa uma mensagem pelo cérebro, sem WhatsApp, e mostra o resultado")
	mostrarVersao := flag.Bool("versao", false, "mostra a versão e sai")
	esqueciSenha := flag.Bool("esqueci-senha", false, "abre, por alguns minutos, a tela para criar uma senha nova do super administrador")
	ouvir := flag.String("ouvir", "", "transcreve um arquivo de áudio do WhatsApp (.ogg) e mostra o texto, para conferir o ouvido")
	flag.Parse()
	log.SetFlags(log.Ldate | log.Ltime)

	if *mostrarVersao {
		fmt.Println(versao)
		return
	}

	// Trabalha sempre na pasta do proprio programa, de onde quer que seja iniciado.
	if exe, err := os.Executable(); err == nil {
		if real, err := filepath.EvalSymlinks(exe); err == nil {
			exe = real
		}
		_ = os.Chdir(filepath.Dir(exe))
	}

	if *esqueciSenha {
		os.Exit(abrirRedefinicao())
	}

	app := novoApp()
	if *ouvir != "" {
		// Teste do ouvido: não abre o banco nem o WhatsApp, só transcreve o arquivo.
		os.Exit(app.ouvirTesteDeLinha(*ouvir))
	}
	app.carregar()
	app.cofre = novoCofreDoSistema()
	app.carregarChaveOpenAI()

	if *simular != "" {
		os.Exit(app.simular(*simular))
	}

	_ = os.WriteFile("atendente.pid", []byte(strconv.Itoa(os.Getpid())), 0o644)
	go app.cuidarDoBanco()
	go app.vigiarIA()
	go app.prepararVoz()
	go ouvidoDoPainel.vigiar()

	if _, err := app.iniciarPainel(); err != nil {
		log.Fatalf("ERRO: não consegui abrir o painel: %v", err)
	}

	ctx, cancelar := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancelar()
	go func() {
		<-ctx.Done() // o programa esta saindo: o navegador escondido do ouvido fecha junto
		ouvidoDoPainel.mu.Lock()
		ouvidoDoPainel.encerrar()
		ouvidoDoPainel.mu.Unlock()
	}()

	if err := app.iniciarWhats(ctx); err != nil {
		app.definirStatus("desconectado", "Desconectado")
		app.definirErro("Não consegui abrir a sessão do WhatsApp: " + err.Error())
	}

	<-ctx.Done()
	log.Printf("Encerrando o atendente...")
	if cli := app.cliente(); cli != nil {
		cli.Disconnect()
	}
}

// cuidarDoBanco faz a copia de seguranca do dia e apaga sessoes vencidas, de hora em hora.
func (a *App) cuidarDoBanco() {
	for {
		agora := time.Now()
		a.banco.copiaDiaria(agora)
		a.banco.limparSessoesVelhas(agora)
		a.banco.apararRegistro(20000)
		time.Sleep(time.Hour)
	}
}

// simular passa uma mensagem pelo mesmo cerebro, sem WhatsApp, e imprime o resultado.
func (a *App) simular(texto string) int {
	a.mu.Lock()
	cfg := a.cfg.clonar()
	a.mu.Unlock()

	sistema, trechos := a.montarSistema(cfg, texto, "")
	entrada := montarEntrada(nil, []string{texto}, nil, nil)
	saida, modo, err := a.cerebro.Responder(modeloDe(cfg), sistema, entrada, texto)
	if err != nil {
		fmt.Println("FALHA:", err)
		return 1
	}
	l := &Lead{Classificacao: "indefinido"}
	aplicarSaida(l, saida)
	l.etiquetasDaIA(saida.Lead.Etiquetas, nil)
	transbordo := saida.Transbordo || (l.Classificacao == "quente" && !cfg.Agenda.Ativa)

	fmt.Println("CEREBRO:", modo)
	fmt.Println("RESPOSTA:", umaLinha(saida.Resposta))
	if saida.Motivo != "" {
		fmt.Printf("TRANSBORDO: %v (motivo: %s)\n", transbordo, umaLinha(saida.Motivo))
	} else {
		fmt.Printf("TRANSBORDO: %v\n", transbordo)
	}
	j, _ := json.Marshal(leadParaIA(l))
	fmt.Println("LEAD:", string(j))
	if saida.Agendamento.Acao != "nenhuma" {
		ag, _ := json.Marshal(saida.Agendamento)
		fmt.Println("AGENDA:", string(ag))
	}
	if len(trechos) > 0 {
		fmt.Printf("MATERIAIS: %d trechos usados\n", len(trechos))
	}
	return 0
}
