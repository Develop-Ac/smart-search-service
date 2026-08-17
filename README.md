# Smart Search Service - Go Microserviço

Microserviço em Go para busca inteligente com fuzzy matching. Processa buscas de produtos e filtragem de uma API REST.

## 📋 Pré-requisitos

- Go 1.21+
- SQLite3
- Docker (opcional)

## 🚀 Instalação Local

### 1. Instalar dependências

```bash
cd smart-search-service
go mod download
```

### 2. Configurar variáveis de ambiente

```bash
cp .env.example .env
```

Edite o `.env` com suas configurações:
```
PORT=8080
DATABASE_URL=../prisma/dev.db
CORS_ORIGIN=http://localhost:3000
```

### 3. Rodar localmente

```bash
go run .
```

O serviço estará disponível em `http://localhost:8080`

## 🐳 Rodar com Docker

### Build e run

```bash
docker-compose up --build
```

### Apenas run

```bash
docker-compose up
```

## 📡 Endpoints da API

### Health Check
```bash
GET /health
```

Resposta:
```json
{
  "status": "ok"
}
```

### Busca Inteligente (POST)

```bash
POST /api/search
Content-Type: application/json

{
  "query": "parabrisa",
  "limit": 50
}
```

Resposta:
```json
{
  "results": [
    {
      "id": "123",
      "name": "Pára-Brisa Dianteiro",
      "proCodigo": "PARA-001",
      "brand": "Original",
      "score": 95.5,
      "startsWithQuery": false,
      "containsQuery": true
    }
  ],
  "total": 1,
  "query": "parabrisa"
}
```

### Busca Inteligente (GET)

```bash
GET /api/search?q=parabrisa&limit=50
```

### Listar Todos os Produtos

```bash
GET /api/products?limit=100
```

### Obter Detalhes de Produto

```bash
GET /api/products/:id
```

## 🧪 Testar com curl

```bash
# Health check
curl http://localhost:8080/health

# Buscar por query
curl http://localhost:8080/api/search?q=pneu

# Buscar com POST
curl -X POST http://localhost:8080/api/search \
  -H "Content-Type: application/json" \
  -d '{"query":"parabrisa","limit":10}'
```

## 🔧 Recursos

- ✅ Fuzzy Matching com Levenshtein Distance
- ✅ Token-based Matching
- ✅ Normalização de texto (acentos, espaços)
- ✅ Scoring inteligente
- ✅ Suporte a CORS
- ✅ Conexão com SQLite
- ✅ Docker ready
- ✅ Error handling

## 📊 Algoritmo de Busca

1. **Normalização**: Remove acentos, converte para maiúscula, normaliza espaços
2. **Busca Inicial**: Busca no banco com LIKE para pré-filtrar
3. **Scoring**: Calcula score baseado em:
   - Prefix matching (começa com a query)
   - Substring matching (contém a query)
   - Fuzzy matching com Levenshtein
   - Token set ratio
   - Token sort ratio
4. **Ordenação**: Ordena por score, depois por startsWithQuery, depois por containsQuery
5. **Filtragem**: Remove resultados abaixo do threshold (2.0)

## 🔗 Integração com Frontend (Next.js)

Exemplo de chamada no React/Next.js:

```typescript
const searchProducts = async (query: string) => {
  const response = await fetch('http://localhost:8080/api/search', {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
    },
    body: JSON.stringify({
      query,
      limit: 50,
    }),
  });
  
  return response.json();
};
```

## 🛠️ Desenvolvimento

### Build binário

```bash
go build -o smart-search-service
```

### Rodar testes (quando adicionados)

```bash
go test ./...
```

## 📝 Notas

- O banco de dados é aberto em modo read-only
- O score máximo é 100
- Limite máximo de resultados é 500 (por segurança)
- O threshold mínimo de score é 2.0

## 📚 Dependências

- `gin-gonic/gin` - Framework HTTP
- `mattn/go-sqlite3` - Driver SQLite
- `jinzhu/gorm` - ORM (importado mas não essencial)
- `joho/godotenv` - Carregador de .env

## ⚙️ Variáveis de Ambiente

| Variável | Default | Descrição |
|----------|---------|-----------|
| `PORT` | 8080 | Porta do servidor |
| `DATABASE_URL` | ../prisma/dev.db | Caminho do banco SQLite |
| `CORS_ORIGIN` | http://localhost:3000 | Origem CORS permitida |

## 📄 Licença

Developed by Develop-Ac
