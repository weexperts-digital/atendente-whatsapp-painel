package main

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAvisoDoCerebroReconheceLoginExpirado(t *testing.T) {
	semLogin := []string{
		"o claude avisou um erro: Failed to authenticate: OAuth session expired and could not be refreshed",
		"o claude avisou um erro: Not logged in · Please run /login",
		"o claude avisou um erro: Invalid API key · Please run /login",
		`API Error: 401 {"type":"error","error":{"type":"authentication_error","message":"Invalid bearer token"}}`,
	}
	for _, falha := range semLogin {
		if avisoDoCerebro(falha) != avisoSemLogin {
			t.Errorf("devia reconhecer login expirado em %q", falha)
		}
	}
	outras := []string{
		"",
		"o claude demorou mais de 90 segundos",
		"o claude não devolveu structured_output",
		"o claude avisou um erro: API Error: 529 Overloaded",
	}
	for _, falha := range outras {
		if aviso := avisoDoCerebro(falha); aviso != "" {
			t.Errorf("%q não é falha de login, mas virou %q", falha, aviso)
		}
	}
}

func TestErroDaIAExplicaLoginExpirado(t *testing.T) {
	w := httptest.NewRecorder()
	erroDaIA(w, errors.New("o claude avisou um erro: Failed to authenticate: OAuth session expired and could not be refreshed"))
	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("código %d, queria 503", w.Code)
	}
	corpo := w.Body.String()
	if !strings.Contains(corpo, "O login do Claude neste computador expirou") {
		t.Errorf("o aviso não explica o login expirado: %s", corpo)
	}
	if strings.Contains(corpo, "OAuth") || strings.Contains(corpo, "Espere um pouco") {
		t.Errorf("o aviso ainda mostra o erro técnico ou manda esperar: %s", corpo)
	}

	w = httptest.NewRecorder()
	erroDaIA(w, errors.New("o claude avisou um erro: API Error: 529 Overloaded"))
	if w.Code != http.StatusBadGateway || !strings.Contains(w.Body.String(), "Espere um pouco") {
		t.Errorf("falha passageira devia continuar pedindo para esperar: %d %s", w.Code, w.Body.String())
	}
}
