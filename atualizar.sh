#!/bin/bash
# Atualiza o atendente para a versão mais nova do GitHub e religa.
# Não mexe na pasta dados/: conversas, configuração e a conexão do WhatsApp continuam.
# Uso: ./atualizar.sh

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

if [ "$ANTES" = "$DEPOIS" ] && [ -x ./atendente ]; then
  echo "O atendente já está na versão mais nova."
  exit 0
fi

# Monta o programa de novo e religa (o instalador faz as duas coisas).
exec ./instalar.sh
