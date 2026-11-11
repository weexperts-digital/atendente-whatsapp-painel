package main

import "testing"

// O numero humano (que recebe os avisos) tambem pode conversar com o assistente.
// So os comandos dele, que comecam com barra, tem tratamento especial.
func TestDecidir(t *testing.T) {
	cfg := Config{
		NumerosHumanos: []string{"5527999991111", "5527999992222"},
		NuncaResponder: []string{"5527988887777", "5527981112222"},
	}
	casos := []struct {
		nome, tel, texto string
		quer             oQueFazer
	}{
		{"cliente comum", "5527999990000", "Oi", fazConversa},
		{"numero humano mandando Oi", "5527999991111", "Oi", fazConversa},
		{"numero humano sem o nono digito", "552799991111", "Oi", fazConversa},
		{"numero humano mandando /lista", "5527999991111", "/lista", fazComando},
		{"numero humano mandando /retomar", "552799991111", "/retomar 5527999990000", fazComando},
		{"segundo numero humano mandando /lista", "5527999992222", "/lista", fazComando},
		{"outro numero mandando /lista", "5527999990000", "/lista", fazConversa},
		{"numero da lista de nunca responder", "5527988887777", "Oi", fazNada},
	}
	for _, c := range casos {
		if tem := decidir(cfg, c.tel, c.texto); tem != c.quer {
			t.Errorf("%s: decidir(%q, %q) = %d, quero %d", c.nome, c.tel, c.texto, tem, c.quer)
		}
	}

	// Quem recebe os avisos pode se colocar na lista de nunca responder: o
	// assistente para de conversar com ele, mas os comandos seguem valendo.
	cfg.NuncaResponder = append(cfg.NuncaResponder, "5527999991111")
	if tem := decidir(cfg, "5527999991111", "Oi"); tem != fazNada {
		t.Errorf("humano na lista de nunca responder mandando Oi = %d, quero %d", tem, fazNada)
	}
	if tem := decidir(cfg, "5527999991111", "/lista"); tem != fazComando {
		t.Errorf("humano na lista de nunca responder mandando /lista = %d, quero %d", tem, fazComando)
	}
}
