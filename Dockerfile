# Build stage
FROM golang:1.24-alpine AS builder

WORKDIR /app

# Instala dependências necessárias
RUN apk add --no-cache gcc musl-dev sqlite-dev

# Copia go.mod e go.sum
COPY go.mod go.sum ./

# Download dependências
RUN go mod download

# Copia código fonte
COPY . .

# Compila aplicação
RUN CGO_ENABLED=1 GOOS=linux go build -a -installsuffix cgo -o smart-search-service .

# Runtime stage
FROM alpine:latest

RUN apk --no-cache add ca-certificates sqlite-libs

WORKDIR /root/

# Copia binário compilado
COPY --from=builder /app/smart-search-service .
COPY --from=builder /app/.env.example .env
COPY --from=builder /app/dev.db ./dev.db

EXPOSE 8080

CMD ["./smart-search-service"]
