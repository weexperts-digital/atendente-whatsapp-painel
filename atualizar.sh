#!/bin/bash
# Atualiza o atendente para a versão mais nova do GitHub e religa.
# Não mexe na pasta dados/: conversas, configuração e a conexão do WhatsApp continuam.
# Uso: ./atualizar.sh

# Tudo fica dentro de uma função para o bash ler o arquivo inteiro antes de começar:
# a atualização troca este próprio arquivo no meio do caminho.
atualizar() {
  cd "$(dirname "$0")" || exit 1

  if ! command -v git >/dev/null 2>&1 || [ ! -d .git ]; then
    echo "Esta pasta não veio do GitHub pelo git, então não dá para atualizar sozinho."
    echo "Baixe o repositório de novo em outra pasta e copie a pasta dados/ para lá."
    exit 1
  fi

  ANTES="$(git rev-parse --short HEAD 2>/dev/null)"
  echo "Buscando a versão mais nova..."
  if ! git pull --ff-only; then
    echo "Não consegui atualizar. Confira a internet."
    echo "Se você mudou arquivos do programa nesta pasta, guarde as mudanças antes de atualizar."
    exit 1
  fi
  DEPOIS="$(git rev-parse --short HEAD 2>/dev/null)"

  # Se nada mudou e o programa montado já é o do código desta pasta, não há o que fazer.
  # (Compara também a versão: o código pode ter sido baixado antes, sem montar o programa.)
  NO_CODIGO="$(sed -n 's/^[[:space:]]*versao[[:space:]]*=[[:space:]]*"\([^"]*\)".*/\1/p' estado.go 2>/dev/null)"
  NO_PROGRAMA=""
  [ -x ./atendente ] && NO_PROGRAMA="$(./atendente -versao 2>/dev/null)"
  if [ "$ANTES" = "$DEPOIS" ] && [ -n "$NO_PROGRAMA" ] && [ "$NO_CODIGO" = "$NO_PROGRAMA" ]; then
    echo "O atendente já está na versão mais nova ($NO_PROGRAMA)."
    exit 0
  fi

  # Monta o programa de novo e religa (o instalador faz as duas coisas).
  exec ./instalar.sh
}

atualizar "$@"
