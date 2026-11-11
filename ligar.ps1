# Liga (ou religa) o atendente de WhatsApp em segundo plano, no Windows.
# Uso (PowerShell):  powershell -ExecutionPolicy Bypass -File .\ligar.ps1

Set-Location -Path $PSScriptRoot

if (-not (Test-Path ".\atendente.exe")) {
    Write-Host "O programa ainda não foi montado nesta pasta. Vou preparar tudo agora."
    & "$PSScriptRoot\instalar.ps1"
    exit $LASTEXITCODE
}

# 1) Para o processo antigo, se ainda estiver rodando.
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

# 2) Guarda o registro anterior e inicia de novo, sem janela.
Remove-Item "porta.txt" -ErrorAction SilentlyContinue
if (Test-Path "atendente.log") { Move-Item -Force "atendente.log" "atendente-anterior.log" }
$novo = Start-Process -FilePath ".\atendente.exe" -WindowStyle Hidden -PassThru `
    -RedirectStandardError "atendente.log" -RedirectStandardOutput "atendente-saida.log"
Set-Content -Path "atendente.pid" -Value $novo.Id

# 3) Espera ate 30 segundos o painel responder.
for ($i = 0; $i -lt 30; $i++) {
    Start-Sleep -Seconds 1
    if (Test-Path "porta.txt") {
        $porta = (Get-Content "porta.txt").Trim()
        try {
            Invoke-WebRequest -UseBasicParsing "http://127.0.0.1:$porta/api/estado" | Out-Null
            Write-Host "Atendente no ar: http://localhost:$porta"
            exit 0
        } catch { }
    }
}

Write-Host "O atendente não respondeu em 30 segundos. Veja o arquivo atendente.log."
exit 1
