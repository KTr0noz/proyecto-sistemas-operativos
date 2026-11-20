# =====================================================================
#  Compila el proyecto para Linux (servidor) y Windows (cliente GUI)
#  Uso (PowerShell, desde la carpeta Proyecto):   .\compilar.ps1
# =====================================================================
$ErrorActionPreference = 'Stop'
Set-Location $PSScriptRoot

$flags = @('-trimpath', '-ldflags', '-s -w')

Remove-Item -Recurse -Force dist -ErrorAction SilentlyContinue
New-Item -ItemType Directory -Force dist\linux-amd64, dist\windows-amd64 | Out-Null

Write-Host '==> Compilando Servidor para Linux (amd64)...'
$env:GOOS = 'linux'; $env:GOARCH = 'amd64'; $env:CGO_ENABLED = '0'
go build @flags -o dist\linux-amd64\servidorOperativos .\servidor
if ($LASTEXITCODE -ne 0) { throw 'Error compilando servidorOperativos para Linux' }
Copy-Item servidor\servidor.conf, servidor\usuarios.db dist\linux-amd64\

$cf = [IO.File]::ReadAllText("$PWD\dist\linux-amd64\servidor.conf") -replace "`r`n", "`n"
[IO.File]::WriteAllText("$PWD\dist\linux-amd64\servidor.conf", $cf, (New-Object Text.UTF8Encoding $false))
$ud = [IO.File]::ReadAllText("$PWD\dist\linux-amd64\usuarios.db") -replace "`r`n", "`n"
[IO.File]::WriteAllText("$PWD\dist\linux-amd64\usuarios.db", $ud, (New-Object Text.UTF8Encoding $false))

Write-Host '==> Compilando Cliente OperGUI (Fyne) para Windows (amd64)...'
$env:GOOS = 'windows'; $env:GOARCH = 'amd64'; $env:CGO_ENABLED = '1'
# -H=windowsgui: app grafica normal de Windows (no abre consola)
go build -trimpath -ldflags '-s -w -H=windowsgui' -o dist\windows-amd64\clienteOperativos.exe .\cliente
if ($LASTEXITCODE -ne 0) { throw 'Error compilando clienteOperativos.exe para Windows' }

Write-Host '==> Compilando herramienta de medicion de latencia para Windows (amd64)...'
$env:CGO_ENABLED = '0'
go build @flags -o dist\windows-amd64\medirLatencia.exe .\medicion
if ($LASTEXITCODE -ne 0) { throw 'Error compilando medirLatencia.exe para Windows' }

Remove-Item Env:GOOS, Env:GOARCH, Env:CGO_ENABLED
Write-Host '==> Compilacion exitosa. Archivos generados:'
Get-ChildItem -Recurse dist -File | Select-Object @{n='Archivo';e={$_.FullName.Replace("$PWD\", '')}}, @{n='Tamano_KB';e={[math]::Round($_.Length/1KB)}} | Format-Table -AutoSize
