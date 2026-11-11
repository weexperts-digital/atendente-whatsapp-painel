#!/bin/bash
# Para o atendente que está rodando em segundo plano.
# Uso: ./parar.sh

cd "$(dirname "$0")" || exit 1

if [ ! -f atendente.pid ]; then
  echo "O atendente não está rodando."
  exit 0
fi

PID="$(<atendente.pid)"
if [ -n "$PID" ] && kill -0 "$PID" 2>/dev/null && ps -p "$PID" -o comm= 2>/dev/null | grep -q "atendente"; then
  kill "$PID" 2>/dev/null
  for _ in 1 2 3 4 5 6 7 8 9 10; do
    kill -0 "$PID" 2>/dev/null || break
    sleep 0.5
  done
  kill -9 "$PID" 2>/dev/null
  echo "Atendente parado."
else
  echo "O atendente não está rodando."
fi
rm -f atendente.pid
