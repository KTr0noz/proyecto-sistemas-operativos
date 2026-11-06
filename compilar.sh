#!/bin/sh
# Compila el proyecto para Linux y Windows (amd64) en dist/  (uso: sh compilar.sh)
set -e
cd "$(dirname "$0")"
export CGO_ENABLED=0

rm -rf dist && mkdir -p dist/linux-amd64 dist/windows-amd64

echo "==> Compilando para Linux (amd64)..."
GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o dist/linux-amd64/servidorOperativos ./servidor
GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o dist/linux-amd64/clienteOperativos  ./cliente
cp servidor/servidor.conf servidor/usuarios.db dist/linux-amd64/
sed -i 's/\r$//' dist/linux-amd64/servidor.conf dist/linux-amd64/usuarios.db

echo "==> Compilando para Windows (amd64)..."
GOOS=windows GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o dist/windows-amd64/clienteOperativos.exe ./cliente
cp servidor/servidor.conf servidor/usuarios.db dist/windows-amd64/

ls -lh dist/*
