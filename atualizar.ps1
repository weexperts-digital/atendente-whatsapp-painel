# Atualiza o atendente para a versão mais nova do GitHub e religa, no Windows.
# Não mexe na pasta dados: conversas, configuração e a conexão do WhatsApp continuam.
# Uso (PowerShell):  powershell -ExecutionPolicy Bypass -File .\atualizar.ps1

Set-Location -Path $PSScriptRoot

if (-not (Get-Command git -ErrorAction SilentlyContinue) -or -not (Test-Path ".git")) {
    Write-Host "Esta pasta não veio do GitHub pelo git, então não dá para atualizar sozinho."
    Write-Host "Baixe o repositório de novo em outra pasta e copie a pasta dados para lá."
    exit 1
}

$antes = (git rev-parse --short HEAD)
Write-Host "Buscando a versão mais nova..."
git pull --ff-only
if ($LASTEXITCODE -ne 0) {
    Write-Host "Não consegui atualizar. Confira a internet."
    Write-Host "Se você mudou arquivos do programa nesta pasta, guarde as mudanças antes de atualizar."
    exit 1
}
$depois = (git rev-parse --short HEAD)

# Se nada mudou e o programa montado já é o do código desta pasta, não há o que fazer.
# (Compara também a versão: o código pode ter sido baixado antes, sem montar o programa.)
$noCodigo = ""
$achado = Select-String -Path "estado.go" -Pattern '^\s*versao\s*=\s*"([^"]*)"' | Select-Object -First 1
if ($achado) { $noCodigo = $achado.Matches[0].Groups[1].Value }
$noPrograma = ""
if (Test-Path ".\atendente.exe") { $noPrograma = (& ".\atendente.exe" -versao) }
if (($antes -eq $depois) -and $noPrograma -and ($noCodigo -eq $noPrograma)) {
    Write-Host "O atendente já está na versão mais nova ($noPrograma)."
    exit 0
}

# No Windows, o programa em uso não pode ser trocado: para o atendente antes de montar de novo.
if (Test-Path "atendente.pid") {
    $antigo = (Get-Content "atendente.pid" -ErrorAction SilentlyContinue | Select-Object -First 1)
    if ($antigo) {
        $proc = Get-Process -Id $antigo -ErrorAction SilentlyContinue
        if ($proc -and $proc.ProcessName -like "atendente*") {
            Stop-Process -Id $antigo -Force -ErrorAction SilentlyContinue
            Start-Sleep -Seconds 1
        }
    }
}

# Monta o programa de novo e religa (o instalador faz as duas coisas).
& "$PSScriptRoot\instalar.ps1"
exit $LASTEXITCODE
