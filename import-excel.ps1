# Script para importar dados do Excel para o banco de dados
param(
    [string]$ExcelFile = "produtos_limpos.xlsx"
)

$projectDir = Split-Path -Parent $MyInvocation.MyCommand.Path

# Resolve o caminho completo do arquivo
$excelPath = Join-Path $projectDir $ExcelFile
if (-not (Test-Path $excelPath)) {
    $excelPath = $ExcelFile
}

# Verifica se o arquivo existe
if (-not (Test-Path $excelPath)) {
    Write-Host "Erro: Arquivo '$ExcelFile' não encontrado!" -ForegroundColor Red
    Write-Host "Procurei em: $excelPath" -ForegroundColor Yellow
    exit 1
}

$excelPath = (Resolve-Path $excelPath).Path
Write-Host "Importando dados de '$excelPath'..." -ForegroundColor Green

# Executa o seed
cd $projectDir
go run cmd/seed-db/main.go -file="$excelPath"

if ($LASTEXITCODE -eq 0) {
    Write-Host "`nImportação concluída com sucesso!" -ForegroundColor Green
} else {
    Write-Host "`nErro durante importação!" -ForegroundColor Red
    exit 1
}
