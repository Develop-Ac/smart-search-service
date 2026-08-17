package main

import (
	"database/sql"
	"fmt"
	"log"
	"sort"
	"strings"

	_ "github.com/lib/pq"
)

type Product struct {
	ID        string
	ProCodigo string
	Name      string
	Category  string
	Brand     string
	Active    bool
}

// Busca produtos por string concatenada nas colunas, ignorando nulos
func SearchProdutosCarros(db *sql.DB, search string, limit int) ([]map[string]interface{}, error) {
	var query string
	var params []interface{}

	if strings.TrimSpace(search) == "" {
		// Se não há termo de busca, retorna todos os registros
		query = `
			SELECT pro_codigo, pro_descricao, referencia,
				carro_1, ano_1, carro_2, ano_2, carro_3, ano_3, carro_4, ano_4,
				carro_5, ano_5, carro_6, ano_6, carro_7, ano_7, carro_8, ano_8,
				carro_9, ano_9, carro_10, ano_10,
				status
			FROM public.produtos_carros
			LIMIT $1
		`
		params = []interface{}{limit}
	} else {
		// Divide a busca em palavras para buscar cada termo
		searchTerms := strings.Fields(strings.ToUpper(search))

		// Constrói condições para cada termo
		var conditions []string
		paramIndex := 1

		for _, term := range searchTerms {
			// concat_ws ignora NULLs e converte qualquer tipo de coluna (inclusive
			// integer, como ano_*) para texto, evitando erro de cast no Postgres.
			condition := fmt.Sprintf(`(
				UPPER(concat_ws(' ',
					pro_codigo, pro_descricao, referencia,
					carro_1, ano_1, carro_2, ano_2, carro_3, ano_3, carro_4, ano_4,
					carro_5, ano_5, carro_6, ano_6, carro_7, ano_7, carro_8, ano_8,
					carro_9, ano_9, carro_10, ano_10)) LIKE $%d)`, paramIndex)

			conditions = append(conditions, condition)
			params = append(params, "%"+strings.ToUpper(term)+"%")
			paramIndex++
		}

		/*
		   Relevância: o que o comprador digitou vem primeiro.

		   Antes não havia ORDER BY nenhum — a ordem era a que o Postgres
		   entregasse (ordem física da tabela). Buscando "p/brisa", um "COLA DE
		   P/BRISA" podia encabeçar a lista à frente de "P/BRISA GOL", que é o
		   que a pessoa procurava.

		   Três degraus, sobre a DESCRIÇÃO (é o que aparece na tela):
		     0 — a descrição COMEÇA com o termo inteiro ("P/BRISA GOL")
		     1 — a descrição CONTÉM o termo, mas não começa ("COLA DE P/BRISA")
		     2 — casou por outra coluna: código, referência ou carro/ano

		   O termo usado aqui é a busca INTEIRA, não cada palavra: quem digita
		   "p/brisa gol" quer o para-brisa do Gol no topo, e ranquear por palavra
		   solta ("gol") jogaria qualquer peça de Gol para a frente.

		   Dentro de cada degrau, ordem alfabética. O desempate final por
		   pro_codigo existe para a ordem ser estável entre chamadas: duas peças
		   com a mesma descrição sairiam em ordem imprevisível, e o portal pagina
		   sobre esta lista — linha trocando de página entre requisições é
		   resultado sumindo aos olhos de quem navega.
		*/
		termoInteiro := strings.ToUpper(strings.TrimSpace(search))

		idxPrefixo := paramIndex
		params = append(params, termoInteiro+"%")
		paramIndex++

		idxContem := paramIndex
		params = append(params, "%"+termoInteiro+"%")
		paramIndex++

		idxLimite := paramIndex
		params = append(params, limit)

		query = fmt.Sprintf(`
			SELECT pro_codigo, pro_descricao, referencia,
				carro_1, ano_1, carro_2, ano_2, carro_3, ano_3, carro_4, ano_4,
				carro_5, ano_5, carro_6, ano_6, carro_7, ano_7, carro_8, ano_8,
				carro_9, ano_9, carro_10, ano_10,
				status
			FROM public.produtos_carros
			WHERE %s
			ORDER BY
				CASE
					WHEN UPPER(COALESCE(pro_descricao, '')) LIKE $%d THEN 0
					WHEN UPPER(COALESCE(pro_descricao, '')) LIKE $%d THEN 1
					ELSE 2
				END,
				pro_descricao ASC,
				pro_codigo ASC
			LIMIT $%d
		`, strings.Join(conditions, " AND "), idxPrefixo, idxContem, idxLimite)
	}

	rows, err := db.Query(query, params...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []map[string]interface{}
	cols, _ := rows.Columns()
	for rows.Next() {
		vals := make([]interface{}, len(cols))
		ptrs := make([]interface{}, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, err
		}
		rowMap := make(map[string]interface{})
		for i, col := range cols {
			rowMap[col] = vals[i]
		}
		results = append(results, rowMap)
	}
	return results, nil
}

func InitializeDB(dbPath string) (*sql.DB, error) {
	fmt.Printf("Connecting to database: %s\n", dbPath)
	
	// Lista de strings de conexão para tentar
	connectionStrings := []string{
		dbPath, // Tenta a string original primeiro
	}
	
	// Se contém o hostname problemático, adiciona alternativas
	if strings.Contains(dbPath, "panel-teste.acacessorios.local") {
		// Tenta com localhost (caso esteja no mesmo servidor)
		altPath1 := strings.Replace(dbPath, "panel-teste.acacessorios.local", "localhost", 1)
		connectionStrings = append(connectionStrings, altPath1)
		
		// Tenta com 127.0.0.1
		altPath2 := strings.Replace(dbPath, "panel-teste.acacessorios.local", "127.0.0.1", 1)
		connectionStrings = append(connectionStrings, altPath2)
		
		// Tenta com IP interno comum do Docker
		altPath3 := strings.Replace(dbPath, "panel-teste.acacessorios.local", "172.17.0.1", 1)
		connectionStrings = append(connectionStrings, altPath3)
	}
	
	var lastErr error
	for i, connStr := range connectionStrings {
		fmt.Printf("Attempt %d: Trying connection: %s\n", i+1, connStr)
		
		db, err := sql.Open("postgres", connStr)
		if err != nil {
			fmt.Printf("Failed to open connection: %v\n", err)
			lastErr = err
			continue
		}

		// Testa conexão
		if err := db.Ping(); err != nil {
			fmt.Printf("Failed to ping database: %v\n", err)
			db.Close()
			lastErr = err
			continue
		}
		
		fmt.Printf("Successfully connected to database!\n")
		return db, nil
	}
	
	return nil, fmt.Errorf("failed to connect to database after %d attempts. Last error: %v", len(connectionStrings), lastErr)
}

func GetProducts(db *sql.DB, limit int) ([]map[string]interface{}, error) {
	// Usa a mesma função SearchProdutosCarros sem filtro
	return SearchProdutosCarros(db, "", limit)
}

func SearchProducts(db *sql.DB, query string, limit int) ([]map[string]interface{}, error) {
	// Usa SearchProdutosCarros com o filtro
	return SearchProdutosCarros(db, query, limit)
}

// SearchProductsFuzzy executa busca com fuzzy matching
func SearchProductsFuzzy(db *sql.DB, query string, limit int) ([]map[string]interface{}, error) {
	// Se não há termo de busca, retorna todos os registros
	if strings.TrimSpace(query) == "" {
		return SearchProdutosCarros(db, "", limit)
	}

	log.Printf("[FUZZY] Starting search for query: '%s', limit: %d", query, limit)

	// Busca todos os produtos (sem filtro SQL, faremos o fuzzy em Go)
	allProducts, err := SearchProdutosCarros(db, "", limit*10) // Busca mais para filtrar depois
	if err != nil {
		return nil, err
	}

	log.Printf("[FUZZY] Retrieved %d products from database", len(allProducts))

	// Aplica fuzzy matching
	var fuzzyResults []SearchResult
	processedCount := 0
	
	for _, product := range allProducts {
		// Extrai campos do produto
		name, _ := product["pro_descricao"].(string)
		code, _ := product["pro_codigo"].(string)
		
		// Calcula score fuzzy
		score := calculateSearchScore(query, name, code)
		processedCount++
		
		// Log de alguns exemplos para debug
		if processedCount <= 5 {
			log.Printf("[FUZZY] Product %d: '%s' (code: %s) -> score: %.2f", processedCount, name, code, score)
		}
		
		// Só inclui se o score for acima do threshold
		if score >= MINIMUM_SCORE_THRESHOLD {
			fuzzyResults = append(fuzzyResults, SearchResult{
				ID:        fmt.Sprintf("%v", product["pro_codigo"]),
				Name:      name,
				ProCodigo: code,
				Score:     score,
			})
		}
	}

	// `%d` e não `%.1f`: MINIMUM_SCORE_THRESHOLD é int, e o verbo errado fazia o
	// `go vet` falhar — ruído que esconde problema de verdade num próximo vet.
	log.Printf("[FUZZY] Found %d results above threshold (%d)", len(fuzzyResults), MINIMUM_SCORE_THRESHOLD)

	// Ordena por score (maior primeiro)
	sort.Slice(fuzzyResults, func(i, j int) bool {
		return fuzzyResults[i].Score > fuzzyResults[j].Score
	})

	// Limita resultados
	if len(fuzzyResults) > limit {
		fuzzyResults = fuzzyResults[:limit]
	}

	// Converte de volta para map[string]interface{} mantendo os dados originais
	var results []map[string]interface{}
	for _, fuzzyResult := range fuzzyResults {
		// Encontra o produto original pelos dados
		for _, product := range allProducts {
			if fmt.Sprintf("%v", product["pro_codigo"]) == fuzzyResult.ID {
				// Adiciona o score ao produto original
				productCopy := make(map[string]interface{})
				for k, v := range product {
					productCopy[k] = v
				}
				productCopy["fuzzy_score"] = fuzzyResult.Score
				results = append(results, productCopy)
				break
			}
		}
	}

	return results, nil
}
