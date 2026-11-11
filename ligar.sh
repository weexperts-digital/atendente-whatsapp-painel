#!/bin/bash
# Liga (ou religa) o atendente de WhatsApp em segundo plano.
# Uso: ./ligar.sh

cd "$(dirname "$0")" || exit 1

if [ ! -x ./atendente ]; then
  echo "O programa ainda não foi montado nesta pasta. Vou preparar tudo agora."
  exec ./instalar.sh
fi

# 1) Para o processo antigo, se ainda estiver rodando.
./parar.sh >/dev/null 2>&1 || true

# 2) Guarda o registro anterior e inicia de novo, solto do terminal.
rm -f porta.txt
[ -f atendente.log ] && mv -f atendente.log atendente-anterior.log
nohup ./atendente > atendente.log 2>&1 < /dev/null &
echo $! > atendente.pid
disown 2>/dev/null || true

# 3) Espera ate 30 segundos o painel responder.
for _ in $(seq 1 30); do
  sleep 1
  if [ -f porta.txt ]; then
    PORTA="$(<porta.txt)"
    if curl -fs "http://127.0.0.1:${PORTA}/api/estado" > /dev/null 2>&1; then
      echo "Atendente no ar: http://localhost:${PORTA}"
      exit 0
    fi
  fi
done

echo "O atendente não respondeu em 30 segundos. Veja o arquivo atendente.log."
exit 1
