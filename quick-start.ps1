# Smart Search Service - Quick Start Script
# Execute este script para rodar tudo automaticamente

# Cores para output
$Green = [Console]::ForegroundColor = 'Green'
$Yellow = [Console]::ForegroundColor = 'Yellow'
$Red = [Console]::ForegroundColor = 'Red'

function Write-Header {
    param([string]$Text)
    Write-Host ""
    Write-Host "========================================" -ForegroundColor Cyan
    Write-Host $Text -ForegroundColor Cyan
    Write-Host "========================================" -ForegroundColor Cyan
    Write-Host ""
}

function Write-Success {
    param([string]$Text)
    Write-Host "✓ $Text" -ForegroundColor Green
}

function Write-Info {
    param([string]$Text)
    Write-Host "ℹ $Text" -ForegroundColor Yellow
}

function Write-Error-Custom {
    param([string]$Text)
    Write-Host "✗ $Text" -ForegroundColor Red
}

Write-Header "Smart Search Service - Quick Start"

# Menu de opções
Write-Host "Selecione uma opção:" -ForegroundColor Cyan
Write-Host ""
Write-Host "1. Rodar localmente com Go (recomendado para desenvolvimento)"
Write-Host "2. Rodar com Docker (recomendado para testes)"
Write-Host "3. Apenas fazer build do Docker (sem rodar)"
Write-Host "4. Parar o serviço (se estiver rodando)"
Write-Host "5. Testar a API"
Write-Host "6. Ver logs do Docker"
Write-Host ""

$option = Read-Host "Digite a opção (1-6)"

switch ($option) {
    "1" {
        Write-Header "Iniciando com Go"
        
        # Verifica se Go está instalado
        try {
            $goVersion = go version
            Write-Success "Go encontrado: $goVersion"
        }
        catch {
            Write-Error-Custom "Go não está instalado ou não está no PATH"
            Write-Host "Baixe em: https://golang.org/dl/"
            exit 1
        }

        # Navega para o diretório do serviço
        Set-Location -Path "smart-search-service" -ErrorAction Stop
        Write-Success "Entrando em: smart-search-service"

        # Verifica .env
        if (!(Test-Path ".env")) {
            Write-Info "Criando arquivo .env..."
            Copy-Item ".env.example" ".env"
            Write-Success "Arquivo .env criado"
        }

        # Download de dependências
        Write-Host ""
        Write-Info "Baixando dependências..."
        go mod download
        Write-Success "Dependências baixadas"

        # Roda o serviço
        Write-Host ""
        Write-Header "🚀 Iniciando o serviço..."
        Write-Host ""
        Write-Host "O serviço estará disponível em: http://localhost:8080" -ForegroundColor Green
        Write-Host "Pressione Ctrl+C para parar" -ForegroundColor Yellow
        Write-Host ""

        go run .
    }

    "2" {
        Write-Header "Iniciando com Docker"

        # Verifica se Docker está instalado
        try {
            docker --version | Out-Null
            Write-Success "Docker encontrado"
        }
        catch {
            Write-Error-Custom "Docker não está instalado"
            Write-Host "Baixe em: https://www.docker.com/products/docker-desktop"
            exit 1
        }

        Set-Location -Path "smart-search-service" -ErrorAction Stop
        Write-Success "Entrando em: smart-search-service"

        # Verifica .env
        if (!(Test-Path ".env")) {
            Write-Info "Criando arquivo .env..."
            Copy-Item ".env.example" ".env"
            Write-Success "Arquivo .env criado"
        }

        Write-Host ""
        Write-Info "Fazendo build do Docker..."
        docker-compose up --build

        Write-Host ""
        Write-Success "Docker iniciado!"
        Write-Host "O serviço está disponível em: http://localhost:8080" -ForegroundColor Green
    }

    "3" {
        Write-Header "Build do Docker"

        Set-Location -Path "smart-search-service" -ErrorAction Stop

        Write-Info "Fazendo build..."
        docker-compose build

        Write-Success "Build concluído!"
        Write-Info "Para rodar, execute: docker-compose up"
    }

    "4" {
        Write-Header "Parando o serviço"

        Set-Location -Path "smart-search-service" -ErrorAction Stop

        Write-Info "Parando containers..."
        docker-compose down

        Write-Success "Serviço parado!"
    }

    "5" {
        Write-Header "Testando a API"

        Write-Host "Testando endpoints..." -ForegroundColor Cyan
        Write-Host ""

        # Health check
        Write-Info "Health Check..."
        try {
            $response = Invoke-WebRequest -Uri "http://localhost:8080/health" -ErrorAction Stop
            Write-Success "✓ Health: $($response.Content)"
        }
        catch {
            Write-Error-Custom "✗ Health check falhou"
            Write-Error-Custom "Certifique-se que o serviço está rodando"
        }

        Write-Host ""

        # Busca
        Write-Info "Busca por 'pneu'..."
        try {
            $response = Invoke-WebRequest -Uri "http://localhost:8080/api/search?q=pneu&limit=5" -ErrorAction Stop
            $data = $response.Content | ConvertFrom-Json
            Write-Success "✓ Encontrados $($data.total) resultados"
            $data.results | ForEach-Object {
                Write-Host "  - $($_.name) (Score: $($_.score.ToString("F1"))%)"
            }
        }
        catch {
            Write-Error-Custom "✗ Busca falhou"
        }

        Write-Host ""
    }

    "6" {
        Write-Header "Logs do Docker"

        Set-Location -Path "smart-search-service" -ErrorAction Stop

        Write-Info "Mostrando logs (Ctrl+C para parar)..."
        docker-compose logs -f
    }

    default {
        Write-Error-Custom "Opção inválida!"
    }
}

Write-Host ""
