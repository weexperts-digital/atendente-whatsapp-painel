#!/bin/bash
# Prepara o atendente neste computador (Mac ou Linux) e liga.
# Baixa o Go para dentro desta pasta se precisar (sem senha de administrador),
# compila o programa e abre o painel.
# Uso: ./instalar.sh

set -e
cd "$(dirname "$0")"

dizer() { printf '\n%s\n' "$*"; }

# 1) Acha um Go que sirva: o desta pasta ou o do computador.
GO=""
if [ -x .go-sdk/go/bin/go ]; then
  GO="$PWD/.go-sdk/go/bin/go"
elif command -v go >/dev/null 2>&1; then
  # O Go do computador serve a partir da versao 1.21 (ele mesmo busca a versao certa).
  MENOR="$(go env GOVERSION 2>/dev/null | sed -E 's/^go1\.([0-9]+).*/\1/')"
  if [ -n "$MENOR" ] && [ "$MENOR" -ge 21 ] 2>/dev/null; then
    GO="$(command -v go)"
  fi
fi

# 2) Se nao achou, baixa o Go oficial para a pasta .go-sdk.
if [ -z "$GO" ]; then
  case "$(uname -s)" in
    Darwin) SISTEMA="darwin" ;;
    Linux) SISTEMA="linux" ;;
    *) echo "Este instalador funciona em Mac e Linux. No Windows, use instalar.ps1."; exit 1 ;;
  esac
  case "$(uname -m)" in
    arm64 | aarch64) CHIP="arm64" ;;
    x86_64 | amd64) CHIP="amd64" ;;
    *) echo "Não reconheci o processador deste computador: $(uname -m)"; exit 1 ;;
  esac
  dizer "Baixando o Go (a linguagem em que o atendente foi escrito). Leva um ou dois minutos..."
  VERSAO="$(curl -fsSL 'https://go.dev/VERSION?m=text' | sed -n 1p)"
  if [ -z "$VERSAO" ]; then
    echo "Não consegui falar com go.dev. Confira a internet e tente de novo."
    exit 1
  fi
  mkdir -p .go-sdk
  curl -fSL --progress-bar "https://go.dev/dl/${VERSAO}.${SISTEMA}-${CHIP}.tar.gz" -o .go-sdk/go.tar.gz
  tar -xzf .go-sdk/go.tar.gz -C .go-sdk
  rm -f .go-sdk/go.tar.gz
  GO="$PWD/.go-sdk/go/bin/go"
fi

# 3) Compila. Sem CGO, para nao depender de mais nada no computador.
dizer "Montando o programa. Na primeira vez leva alguns minutos..."
CGO_ENABLED=0 "$GO" build -o atendente .

# 4) O cerebro e o Claude Code. Sem ele o atendente liga em modo teste.
if ! command -v claude >/dev/null 2>&1 && [ ! -x "$HOME/.local/bin/claude" ]; then
  dizer "Aviso: não achei o Claude Code neste computador."
  echo "O painel abre mesmo assim. Para o assistente responder de verdade, escolha um caminho:"
  echo "  1. Instale o Claude Code em https://claude.com/claude-code e rode ./ligar.sh de novo. Depois clique em Conectar a IA no painel."
  echo "  2. Ou use o ChatGPT: no painel, em Conectar a IA, aba ChatGPT, cole uma chave da OpenAI."
fi

# 5) Liga e abre o painel.
./ligar.sh
if [ -f porta.txt ]; then
  ENDERECO="http://localhost:$(cat porta.txt)"
  if command -v open >/dev/null 2>&1; then
    open "$ENDERECO" || true
  elif command -v xdg-open >/dev/null 2>&1; then
    xdg-open "$ENDERECO" >/dev/null 2>&1 || true
  fi
fi
