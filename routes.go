package main

import (
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
)

type SearchRequest struct {
	Query string `json:"query" binding:"required"`
	Limit int    `json:"limit"`
}

func setupRoutes(router *gin.Engine, db *sql.DB, searchMode string) {
	// Health check
	router.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	// Busca inteligente
	router.POST("/api/search", func(c *gin.Context) {
		handleSearch(c, db)
	})

	// Busca por query parameter (GET)
	router.GET("/api/search", func(c *gin.Context) {
		query := c.Query("q")
		if query == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Query parameter 'q' is required"})
			return
		}

		req := SearchRequest{
			Query: query,
			Limit: 50,
		}

		if limitStr := c.Query("limit"); limitStr != "" {
			if limit, err := strconv.Atoi(limitStr); err == nil && limit > 0 {
				req.Limit = limit
			}
		}

		performSearch(c, db, req)
	})

	// Busca por carros/produtos concatenados
	router.POST("/api/search-carros", func(c *gin.Context) {
		handleSearchCarros(c, db, searchMode)
	})
	router.GET("/api/search-carros", func(c *gin.Context) {
		query := c.Query("q")
		if query == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Query parameter 'q' is required"})
			return
		}
		req := SearchRequest{
			Query: query,
			Limit: 50,
		}
		if limitStr := c.Query("limit"); limitStr != "" {
			if limit, err := strconv.Atoi(limitStr); err == nil && limit > 0 {
				req.Limit = limit
			}
		}
		performSearchProdutosCarros(c, db, req, searchMode)
	})

	// Listar todos os produtos
	router.GET("/api/products", func(c *gin.Context) {
		limit := 100
		if limitStr := c.Query("limit"); limitStr != "" {
			if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
				limit = l
			}
		}

		products, err := GetProducts(db, limit)
		if err != nil {
			log.Printf("[ERROR] GetProducts failed: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Erro ao buscar produtos: %v", err)})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"products": products,
			"total":    len(products),
		})
	})
}

// Handler para POST /api/search-carros
func handleSearchCarros(c *gin.Context, db *sql.DB, searchMode string) {
	var req SearchRequest
	if err := c.BindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Request body inválido"})
		return
	}
	if strings.TrimSpace(req.Query) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Query não pode estar vazia"})
		return
	}
	if req.Limit <= 0 {
		req.Limit = 50
	}
	if req.Limit > 500 {
		req.Limit = 500
	}
	performSearchProdutosCarros(c, db, req, searchMode)
}

// Handler para busca na tabela produtos_carros
func performSearchProdutosCarros(c *gin.Context, db *sql.DB, req SearchRequest, searchMode string) {
	var results []map[string]interface{}
	var err error

	if searchMode == "fuzzy" {
		results, err = SearchProductsFuzzy(db, req.Query, req.Limit)
	} else {
		results, err = SearchProdutosCarros(db, req.Query, req.Limit)
	}

	if err != nil {
		log.Printf("[ERROR] Search failed (mode: %s): %v", searchMode, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Erro ao buscar produtos carros: %v", err)})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"results": results,
		"total":   len(results),
		"query":   req.Query,
	})
}

func handleSearch(c *gin.Context, db *sql.DB) {
	var req SearchRequest

	if err := c.BindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Request body inválido"})
		return
	}

	if strings.TrimSpace(req.Query) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Query não pode estar vazia"})
		return
	}

	if req.Limit <= 0 {
		req.Limit = 50
	}
	if req.Limit > 500 {
		req.Limit = 500
	}

	performSearch(c, db, req)
}

func performSearch(c *gin.Context, db *sql.DB, req SearchRequest) {
	// Busca usando a mesma função SearchProducts que agora retorna map
	results, err := SearchProducts(db, req.Query, req.Limit)
	if err != nil {
		log.Printf("[ERROR] SearchProducts failed: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Erro ao buscar produtos: %v", err)})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"results": results,
		"total":   len(results),
		"query":   req.Query,
	})
}
