package main

import (
	"fmt"
	"log"
	"os"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
)

func init() {
	// Carrega variáveis de ambiente
	godotenv.Load()
}

func main() {
	// Obtém configurações
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	dbPath := os.Getenv("DATABASE_URL")
	if dbPath == "" {
		// Tenta primeiro a variável POSTGRES_URL (para produção)
		dbPath = os.Getenv("POSTGRES_URL") 
		if dbPath == "" {
			// Fallback para desenvolvimento local
			dbPath = "postgres://intranet:Ac@2025acesso@panel-teste.acacessorios.local:5555/intranet?sslmode=disable"
		}
	}
	
	log.Printf("Database path: %s", dbPath)

	corsOrigin := os.Getenv("CORS_ORIGIN")
	if corsOrigin == "" {
		corsOrigin = "http://localhost:3000"
	}

	// Configura modo de busca
	searchMode := os.Getenv("SEARCH_MODE")
	if searchMode == "" {
		searchMode = "sql" // Padrão: sql
	}
	log.Printf("Search mode: %s", searchMode)

	/*
	   Dicionário de abreviações (ver `abreviacoes.go`).

	   Sem o arquivo o serviço sobe mesmo assim, com o dicionário embutido:
	   a busca fica menos esperta, não fora do ar — e o log diz o porquê.
	*/
	caminhoAbreviacoes := os.Getenv("ABREVIACOES_PATH")
	if caminhoAbreviacoes == "" {
		caminhoAbreviacoes = "Abreviações.xlsx"
	}
	if d, err := CarregarAbreviacoes(caminhoAbreviacoes); err != nil {
		log.Printf("[ABREVIACOES] %v — seguindo só com o dicionário embutido", err)
	} else {
		dicionario = d
	}

	// Inicializa banco de dados
	db, err := InitializeDB(dbPath)
	if err != nil {
		log.Fatalf("Erro ao conectar ao banco de dados: %v", err)
	}
	defer db.Close()

	// Inicializa router Gin
	router := gin.Default()

	// Middleware CORS
	router.Use(CORSMiddleware(corsOrigin))

	// Rotas
	setupRoutes(router, db, searchMode)

	// Inicia servidor
	addr := fmt.Sprintf(":%s", port)
	log.Printf("Servidor iniciado em http://localhost:%s", port)
	if err := router.Run(addr); err != nil {
		log.Fatalf("Erro ao iniciar servidor: %v", err)
	}
}
