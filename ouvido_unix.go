//go:build !windows

package main

import (
	"io"
	"os"
	"os/exec"
)

// amarrarNavegador liga a vida do navegador escondido a deste programa (Mac e Linux).
// O navegador e aberto com um canal de controle (--remote-debugging-pipe) cujas pontas
// ficam com este programa. Se o programa sair de qualquer jeito, ate morto a forca, o
// sistema fecha o canal e o navegador sai sozinho. O canal nao abre porta de rede.
// Devolve o que rodar logo depois de iniciar o navegador.
func amarrarNavegador(cmd *exec.Cmd) (depoisDeIniciar func(), err error) {
	leitura, nossaEscrita, err := os.Pipe() // o navegador le deste (descritor 3)
	if err != nil {
		return nil, err
	}
	nossaLeitura, escrita, err := os.Pipe() // o navegador escreve neste (descritor 4)
	if err != nil {
		leitura.Close()
		nossaEscrita.Close()
		return nil, err
	}
	cmd.Args = append(cmd.Args[:len(cmd.Args)-1:len(cmd.Args)-1], "--remote-debugging-pipe", cmd.Args[len(cmd.Args)-1])
	cmd.ExtraFiles = []*os.File{leitura, escrita}
	return func() {
		// As pontas do navegador ja foram herdadas: aqui elas fecham.
		leitura.Close()
		escrita.Close()
		// A ponta de escrita fica aberta e parada ate o programa sair. A de leitura so
		// esvazia o que o navegador escrever, para ele nunca travar.
		go func() {
			_, _ = io.Copy(io.Discard, nossaLeitura)
			nossaLeitura.Close()
			nossaEscrita.Close()
		}()
	}, nil
}
