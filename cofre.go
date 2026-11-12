package main

import (
	"errors"
	"os"
	"path/filepath"
	"sync"

	"github.com/zalando/go-keyring"
)

// Cofre: onde a chave da OpenAI fica guardada. E o cofre de senhas do proprio sistema
// (Keychain no Mac, Gerenciador de Credenciais no Windows, Secret Service no Linux).
// A chave nunca vai para arquivo do projeto, para o banco de dados nem para o log.

const (
	servicoDoCofre = "Atendente IA"
	chaveDaOpenAI  = "chave da OpenAI"
)

type cofre interface {
	ler(nome string) (string, error) // "" e nil quando nao ha nada guardado
	guardar(nome, valor string) error
	apagar(nome string) error
}

// cofreDoSistema guarda um segredo por pasta do atendente: duas copias do programa no
// mesmo computador (a de verdade e uma de teste) nao enxergam a chave uma da outra.
type cofreDoSistema struct{ pasta string }

func novoCofreDoSistema() *cofreDoSistema {
	pasta, err := filepath.Abs(pastaDados)
	if err != nil {
		pasta, _ = os.Getwd()
	}
	return &cofreDoSistema{pasta: pasta}
}

func (c *cofreDoSistema) conta(nome string) string { return nome + " em " + c.pasta }

func (c *cofreDoSistema) ler(nome string) (string, error) {
	valor, err := keyring.Get(servicoDoCofre, c.conta(nome))
	if errors.Is(err, keyring.ErrNotFound) {
		return "", nil
	}
	return valor, err
}

func (c *cofreDoSistema) guardar(nome, valor string) error {
	return keyring.Set(servicoDoCofre, c.conta(nome), valor)
}

func (c *cofreDoSistema) apagar(nome string) error {
	err := keyring.Delete(servicoDoCofre, c.conta(nome))
	if errors.Is(err, keyring.ErrNotFound) {
		return nil
	}
	return err
}

// cofreDeMemoria e o cofre dos testes: nada sai do programa.
type cofreDeMemoria struct {
	mu      sync.Mutex
	valores map[string]string
	falha   error // para testar o computador sem cofre
}

func (c *cofreDeMemoria) ler(nome string) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.valores[nome], c.falha
}

func (c *cofreDeMemoria) guardar(nome, valor string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.falha != nil {
		return c.falha
	}
	if c.valores == nil {
		c.valores = map[string]string{}
	}
	c.valores[nome] = valor
	return nil
}

func (c *cofreDeMemoria) apagar(nome string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.falha != nil {
		return c.falha
	}
	delete(c.valores, nome)
	return nil
}
