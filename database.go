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

// apenasDigitos diz se a busca é um código, e não texto: só dígitos, nada de
// letra, espaço ou traço. Serve para decidir o ranking — quem digita "31500"
// quer o produto 31500, quem digita "gol 2015" quer para-brisa de Gol.
func apenasDigitos(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
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
			//
			// O pro_codigo aparece de novo, sozinho, no segundo LIKE: dentro do
			// concat_ws ele fica colado no vizinho (".. 31500 PARABRISA ..") e o
			// código inteiro casa, mas é o mesmo LIKE frouxo que casa qualquer
			// ano; testá-lo à parte deixa o casamento por código explícito e
			// independente das outras colunas.
			condition := fmt.Sprintf(`(
				UPPER(concat_ws(' ',
					pro_codigo, pro_descricao, referencia,
					carro_1, ano_1, carro_2, ano_2, carro_3, ano_3, carro_4, ano_4,
					carro_5, ano_5, carro_6, ano_6, carro_7, ano_7, carro_8, ano_8,
					carro_9, ano_9, carro_10, ano_10)) LIKE $%d
				OR UPPER(COALESCE(CAST(pro_codigo AS TEXT), '')) LIKE $%d)`, paramIndex, paramIndex)

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

		/*
		   Quem digita só números está digitando um código, não uma descrição.

		   Nenhuma descrição começa por "31500", então o ranking por descrição
		   acima empata TODOS os resultados no degrau 2 e a ordem volta a ser a
		   que o Postgres entregar — o próprio código procurado podia sair no
		   meio da lista, atrás de peças que casaram só pelo ano (ano_1 = 2015
		   casa com quem buscou "2015" e também com quem buscou "15").

		   Para busca numérica o ranking passa a ser sobre o pro_codigo:
		     0 — é exatamente o código digitado
		     1 — o código COMEÇA com o que foi digitado
		     2 — o código CONTÉM o que foi digitado
		     3 — casou por outra coluna (descrição, referência, carro/ano)

		   e o desempate é o próprio pro_codigo em ordem crescente, que é a
		   ordem em que o comprador espera ler uma lista de códigos.
		*/
		// Cada ramo registra só os parâmetros que o seu ORDER BY usa: um $n que
		// o SQL nunca referencia deixa o Postgres sem como inferir o tipo e a
		// query morre em "could not determine data type of parameter".
		var ordenacao string
		if apenasDigitos(termoInteiro) {
			idxExato := paramIndex
			params = append(params, termoInteiro)
			paramIndex++

			idxPrefixo := paramIndex
			params = append(params, termoInteiro+"%")
			paramIndex++

			idxContem := paramIndex
			params = append(params, "%"+termoInteiro+"%")
			paramIndex++

			ordenacao = fmt.Sprintf(`
				CASE
					WHEN UPPER(COALESCE(CAST(pro_codigo AS TEXT), '')) = $%d THEN 0
					WHEN UPPER(COALESCE(CAST(pro_codigo AS TEXT), '')) LIKE $%d THEN 1
					WHEN UPPER(COALESCE(CAST(pro_codigo AS TEXT), '')) LIKE $%d THEN 2
					ELSE 3
				END,
				pro_codigo ASC,
				pro_descricao ASC`, idxExato, idxPrefixo, idxContem)
		} else {
			idxPrefixo := paramIndex
			params = append(params, termoInteiro+"%")
			paramIndex++

			idxContem := paramIndex
			params = append(params, "%"+termoInteiro+"%")
			paramIndex++

			ordenacao = fmt.Sprintf(`
				CASE
					WHEN UPPER(COALESCE(pro_descricao, '')) LIKE $%d THEN 0
					WHEN UPPER(COALESCE(pro_descricao, '')) LIKE $%d THEN 1
					ELSE 2
				END,
				pro_descricao ASC,
				pro_codigo ASC`, idxPrefixo, idxContem)
		}

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
			ORDER BY %s
			LIMIT $%d
		`, strings.Join(conditions, " AND "), ordenacao, idxLimite)
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

	/*
	   Código digitado não passa pelo fuzzy.

	   O fuzzy pontua contra a DESCRIÇÃO, e busca por código não sobrevive a
	   isso duas vezes: os candidatos vêm de um SELECT sem WHERE (as primeiras
	   limit*10 linhas da tabela), então o produto 31500 provavelmente nem entra
	   na lista; e mesmo entrando, "31500" contra "P/BRISA GOL" é distância de
	   edição pura — o código certo ficaria abaixo de descrições que só têm
	   dígitos parecidos.

	   Busca numérica vai pelo SQL, que filtra a tabela inteira e já entrega
	   ordenado pelo pro_codigo encontrado.
	*/
	if apenasDigitos(query) {
		log.Printf("[FUZZY] Query '%s' é numérica: usando busca SQL por pro_codigo", query)
		return SearchProdutosCarros(db, query, limit)
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
