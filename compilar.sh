#!/bin/sh
# Compila el servidor Linux y el cliente Windows (amd64) en dist/  (uso: sh compilar.sh)
# El cliente usa Fyne (CGO), por eso necesita el compilador cruzado x86_64-w64-mingw32-gcc (mingw-w64)
set -e
cd "$(dirname "$0")"

rm -rf dist/linux-amd64 && mkdir -p dist/linux-amd64 dist/windows-amd64

echo "==> Compilando Servidor para Linux (amd64)..."
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags="-s -w" -o dist/linux-amd64/servidorOperativos ./servidor
cp servidor/servidor.conf servidor/usuarios.db dist/linux-amd64/
sed -i 's/\r$//' dist/linux-amd64/servidor.conf dist/linux-amd64/usuarios.db

echo "==> Compilando Cliente OperGUI (Fyne) para Windows (amd64)..."
# -H=windowsgui: app grafica normal de Windows (no abre consola)
CGO_ENABLED=1 GOOS=windows GOARCH=amd64 CC=x86_64-w64-mingw32-gcc go build -trimpath -ldflags="-s -w -H=windowsgui" -o dist/windows-amd64/clienteOperativos.exe ./cliente

ls -lh dist/linux-amd64/ dist/windows-amd64/clienteOperativos.exe
