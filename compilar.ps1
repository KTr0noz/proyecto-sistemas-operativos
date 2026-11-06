# =====================================================================
#  Compila el proyecto para Linux y Windows (amd64) en la carpeta dist\
#  Uso (PowerShell, desde la carpeta Proyecto):   .\compilar.ps1
# =====================================================================
$ErrorActionPreference = 'Stop'
Set-Location $PSScriptRoot

$env:CGO_ENABLED = '0'
$flags = @('-trimpath', '-ldflags', '-s -w')

Remove-Item -Recurse -Force dist -ErrorAction SilentlyContinue
New-Item -ItemType Directory -Force dist\linux-amd64, dist\windows-amd64 | Out-Null

Write-Host '==> Compilando Servidor y Cliente para Linux (amd64)...'
$env:GOOS = 'linux'; $env:GOARCH = 'amd64'
go build @flags -o dist\linux-amd64\servidorOperativos .\servidor
go build @flags -o dist\linux-amd64\clienteOperativos  .\cliente
Copy-Item servidor\servidor.conf, servidor\usuarios.db dist\linux-amd64\

# Normalizar finales de linea LF para Linux
$cf = [IO.File]::ReadAllText("$PWD\dist\linux-amd64\servidor.conf") -replace "`r`n", "`n"
[IO.File]::WriteAllText("$PWD\dist\linux-amd64\servidor.conf", $cf, (New-Object Text.UTF8Encoding $false))
$ud = [IO.File]::ReadAllText("$PWD\dist\linux-amd64\usuarios.db") -replace "`r`n", "`n"
[IO.File]::WriteAllText("$PWD\dist\linux-amd64\usuarios.db", $ud, (New-Object Text.UTF8Encoding $false))

Write-Host '==> Compilando Cliente y Servidor para Windows (amd64)...'
$env:GOOS = 'windows'; $env:GOARCH = 'amd64'
go build @flags -o dist\windows-amd64\clienteOperativos.exe .\cliente
go build @flags -o dist\windows-amd64\servidorOperativos.exe .\servidor
Copy-Item servidor\servidor.conf, servidor\usuarios.db dist\windows-amd64\

Remove-Item Env:GOOS, Env:GOARCH, Env:CGO_ENABLED
Write-Host '==> Compilacion exitosa. Archivos generados:'
Get-ChildItem -Recurse dist -File | Select-Object @{n='Archivo';e={$_.FullName.Replace("$PWD\", '')}}, @{n='Tamano_KB';e={[math]::Round($_.Length/1KB)}} | Format-Table -AutoSize
