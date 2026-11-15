#!/bin/sh
# Compila el servidor Linux (amd64) en dist/  (uso: sh compilar.sh)
set -e
cd "$(dirname "$0")"

rm -rf dist/linux-amd64 && mkdir -p dist/linux-amd64

echo "==> Compilando Servidor para Linux (amd64)..."
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o dist/linux-amd64/servidorOperativos ./servidor
cp servidor/servidor.conf servidor/usuarios.db dist/linux-amd64/
sed -i 's/\r$//' dist/linux-amd64/servidor.conf dist/linux-amd64/usuarios.db

ls -lh dist/linux-amd64/
