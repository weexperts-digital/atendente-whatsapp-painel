# Prepara o atendente no Windows e liga.
# Baixa o Go para dentro desta pasta se precisar, compila e abre o painel.
# Uso (PowerShell):  powershell -ExecutionPolicy Bypass -File .\instalar.ps1

$ErrorActionPreference = "Stop"
Set-Location -Path $PSScriptRoot

# 1) Acha um Go que sirva: o desta pasta ou o do computador.
$go = $null
if (Test-Path ".go-sdk\go\bin\go.exe") {
    $go = (Resolve-Path ".go-sdk\go\bin\go.exe").Path
} elseif (Get-Command go -ErrorAction SilentlyContinue) {
    $go = (Get-Command go).Source
}

# 2) Se nao achou, baixa o Go oficial para a pasta .go-sdk.
if (-not $go) {
    Write-Host "Baixando o Go (a linguagem em que o atendente foi escrito). Leva um ou dois minutos..."
    $versao = ((Invoke-WebRequest -UseBasicParsing "https://go.dev/VERSION?m=text").Content -split "`n")[0].Trim()
    $chip = if ($env:PROCESSOR_ARCHITECTURE -eq "ARM64") { "arm64" } else { "amd64" }
    New-Item -ItemType Directory -Force -Path ".go-sdk" | Out-Null
    Invoke-WebRequest -UseBasicParsing "https://go.dev/dl/$versao.windows-$chip.zip" -OutFile ".go-sdk\go.zip"
    Expand-Archive -Path ".go-sdk\go.zip" -DestinationPath ".go-sdk" -Force
    Remove-Item ".go-sdk\go.zip"
    $go = (Resolve-Path ".go-sdk\go\bin\go.exe").Path
}

# 3) Compila. Sem CGO, para nao depender de mais nada no computador.
Write-Host "Montando o programa. Na primeira vez leva alguns minutos..."
$env:CGO_ENABLED = "0"
& $go build -o atendente.exe .
if ($LASTEXITCODE -ne 0) { throw "Não consegui montar o programa." }

# 4) O cerebro e o Claude Code. Sem ele o atendente liga em modo teste.
if (-not (Get-Command claude -ErrorAction SilentlyContinue) -and -not (Test-Path "$HOME\.local\bin\claude.exe")) {
    Write-Host ""
    Write-Host "Aviso: não achei o Claude Code neste computador."
    Write-Host "O painel abre mesmo assim. Para o assistente responder de verdade, escolha um caminho:"
    Write-Host "  1. Instale o Claude Code em https://claude.com/claude-code e rode ligar.ps1 de novo. Depois clique em Conectar a IA no painel."
    Write-Host "  2. Ou use o ChatGPT: no painel, em Conectar a IA, aba ChatGPT, cole uma chave da OpenAI."
}

# 5) Liga e abre o painel.
& "$PSScriptRoot\ligar.ps1"
if (Test-Path "porta.txt") {
    Start-Process ("http://localhost:" + (Get-Content "porta.txt").Trim())
}
