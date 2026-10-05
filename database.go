package main

import (
	"database/sql"
	"fmt"
	"log"
	"regexp"
	"sort"
	"strings"
	"sync"

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
// Colunas devolvidas ao chamador, na ordem em que o portal espera lê-las.
//
// Tudo sai como TEXT. O UNION exige o mesmo tipo em cada posição dos dois
// lados, e os catálogos discordam: `produtos_carros.pro_codigo` é inteiro,
// `products.pro_codigo` é texto (há código com letra no catálogo). TEXT é o
// único acordo possível — e o consumidor já aceita os dois: `comoTexto`, no
// smart-search.service.ts do back, coage número e string para o mesmo texto.
const colunasResultado = `pro_codigo, pro_descricao, referencia, brand,
	carro_1, ano_1,
	carro_2, ano_2,
	carro_3, ano_3,
	carro_4, ano_4,
	carro_5, ano_5,
	carro_6, ano_6,
	carro_7, ano_7,
	carro_8, ano_8,
	carro_9, ano_9,
	carro_10, ano_10,
	status, origem`

// Ordena código como número sem precisar convertê-lo: entre códigos de dígitos,
// o mais curto vem antes e, no mesmo tamanho, a ordem textual já é a numérica.
// Um CAST para inteiro estouraria nos códigos com letra, que caem no 99 e vão
// para o fim da lista.
const ordemPorCodigo = `CASE WHEN pro_codigo ~ '^[0-9]+$' THEN LENGTH(pro_codigo) ELSE 99 END,
				pro_codigo ASC`

/*
   Busca nas DUAS tabelas, com `products` completando o que falta.

   A `produtos_carros` é o catálogo com aplicação (carro/ano), mas está
   incompleta: o código 177 (GRAMPO FORRO PORTA UNO) não existe nela, embora
   exista em `products`. Buscar só nela é devolver "tudo menos o item" para
   toda peça nessa situação.

   O UNION resolve pelo código: quando a peça está nas duas vale a linha de
   `produtos_carros`, a única que traz carro/ano; quando só existe em
   `products`, ela entra com as colunas de aplicação nulas. A coluna `origem`
   diz de qual tabela cada linha veio.

   O DISTINCT ON é o desempate — ordenado por `prioridade_origem`, fica a linha
   de `produtos_carros` (0) e some a de `products` (1). Ele mora num CTE
   separado porque o DISTINCT ON obriga o ORDER BY a começar pelo pro_codigo,
   que não é a ordem de relevância que o resultado precisa ter no fim.
*/
const consultaUnificada = `
	WITH base AS (
		SELECT
			CAST(pc.pro_codigo AS TEXT) AS pro_codigo,
			CAST(pc.pro_descricao AS TEXT) AS pro_descricao,
			CAST(pc.referencia AS TEXT) AS referencia,
			-- brand só existe em products. Como o DISTINCT ON prefere a linha de
			-- produtos_carros (prioridade 0), buscar a marca aqui pelo pro_codigo
			-- é o que impede a coluna de sair nula para toda peça que está nos
			-- dois catálogos. Subconsulta, e não JOIN: código repetido em
			-- products multiplicaria a linha antes da deduplicação.
			(SELECT CAST(pb.brand AS TEXT)
			 FROM public.products pb
			 WHERE pb.pro_codigo = CAST(pc.pro_codigo AS TEXT)
			 LIMIT 1) AS brand,
			CAST(pc.carro_1 AS TEXT) AS carro_1, CAST(pc.ano_1 AS TEXT) AS ano_1,
			CAST(pc.carro_2 AS TEXT) AS carro_2, CAST(pc.ano_2 AS TEXT) AS ano_2,
			CAST(pc.carro_3 AS TEXT) AS carro_3, CAST(pc.ano_3 AS TEXT) AS ano_3,
			CAST(pc.carro_4 AS TEXT) AS carro_4, CAST(pc.ano_4 AS TEXT) AS ano_4,
			CAST(pc.carro_5 AS TEXT) AS carro_5, CAST(pc.ano_5 AS TEXT) AS ano_5,
			CAST(pc.carro_6 AS TEXT) AS carro_6, CAST(pc.ano_6 AS TEXT) AS ano_6,
			CAST(pc.carro_7 AS TEXT) AS carro_7, CAST(pc.ano_7 AS TEXT) AS ano_7,
			CAST(pc.carro_8 AS TEXT) AS carro_8, CAST(pc.ano_8 AS TEXT) AS ano_8,
			CAST(pc.carro_9 AS TEXT) AS carro_9, CAST(pc.ano_9 AS TEXT) AS ano_9,
			CAST(pc.carro_10 AS TEXT) AS carro_10, CAST(pc.ano_10 AS TEXT) AS ano_10,
			CAST(pc.status AS TEXT) AS status,
			CAST('produtos_carros' AS TEXT) AS origem,
			0 AS prioridade_origem
		FROM public.produtos_carros pc
		WHERE %s
		UNION ALL
		SELECT
			CAST(p.pro_codigo AS TEXT) AS pro_codigo,
			CAST(p.name AS TEXT) AS pro_descricao,
			NULL::text AS referencia,
			CAST(p.brand AS TEXT) AS brand,
			NULL::text AS carro_1, NULL::text AS ano_1,
			NULL::text AS carro_2, NULL::text AS ano_2,
			NULL::text AS carro_3, NULL::text AS ano_3,
			NULL::text AS carro_4, NULL::text AS ano_4,
			NULL::text AS carro_5, NULL::text AS ano_5,
			NULL::text AS carro_6, NULL::text AS ano_6,
			NULL::text AS carro_7, NULL::text AS ano_7,
			NULL::text AS carro_8, NULL::text AS ano_8,
			NULL::text AS carro_9, NULL::text AS ano_9,
			NULL::text AS carro_10, NULL::text AS ano_10,
			NULL::text AS status,
			CAST('products' AS TEXT) AS origem,
			1 AS prioridade_origem
		FROM public.products p
		WHERE p.active IS TRUE AND %s
	),
	unicos AS (
		SELECT DISTINCT ON (pro_codigo) *
		FROM base
		ORDER BY pro_codigo, prioridade_origem
	)
	SELECT ` + colunasResultado + `
	FROM unicos
	ORDER BY %s
	LIMIT $%d
`

/*
   `nomeCelta` é coluna nova e pode não existir ainda em todo ambiente.

   Referenciá-la direto quebraria a busca INTEIRA com "column does not exist"
   onde a migração não passou — e o serviço não tem como distinguir isso de um
   banco fora do ar. Por isso o catálogo é consultado uma vez no primeiro uso e
   a coluna só entra no concat_ws onde realmente existe.

   O nome vai entre aspas porque o Prisma cria coluna camelCase citada: sem as
   aspas o Postgres procuraria `nomecelta`, tudo minúsculo, e não acharia.
*/
var (
	deteccaoUmaVez     sync.Once
	nomeCeltaEmCarros  bool
	nomeCeltaEmProduct bool
)

func colunaExiste(db *sql.DB, tabela, coluna string) bool {
	var existe bool
	err := db.QueryRow(`
		SELECT EXISTS (
			SELECT 1 FROM information_schema.columns
			WHERE table_schema = 'public' AND table_name = $1 AND column_name = $2
		)`, tabela, coluna).Scan(&existe)
	if err != nil {
		log.Printf("[SCHEMA] não deu para checar %s.%s (%v) — coluna ignorada", tabela, coluna, err)
		return false
	}
	return existe
}

func detectarColunas(db *sql.DB) {
	deteccaoUmaVez.Do(func() {
		nomeCeltaEmCarros = colunaExiste(db, "produtos_carros", "nomeCelta")
		nomeCeltaEmProduct = colunaExiste(db, "products", "nomeCelta")
		log.Printf("[SCHEMA] nomeCelta: produtos_carros=%v products=%v",
			nomeCeltaEmCarros, nomeCeltaEmProduct)
	})
}

/*
   condicaoDeForma é uma forma do dicionário já registrada como parâmetro,
   pronta para virar `expr LIKE $n` — ou, para forma curta, `expr ~ $n` com
   casamento de palavra inteira.

   "LE" (lado esquerdo), "DT" (dianteiro), "C/" (com) têm duas letras: num
   `LIKE '%LE%'` casariam LENTE, VOLANTE, PALHETA — metade do catálogo. Com
   `\m` e `\M` (início e fim de palavra no regex do Postgres) "LE" só casa o
   "LE" de "TRAS.LE" e "LD/LE", que é o que o cadastro escreve. O padrão
   inteiro vai no parâmetro, montado em Go: a forma escapada
   (`regexp.QuoteMeta` — "C." tem ponto, que é curinga) entre as âncoras.
   Nada de `'\m' || $n` no SQL, cujo sentido dependeria de
   `standard_conforming_strings`.
*/
type condicaoDeForma struct {
	idx            int
	palavraInteira bool // forma curta: só palavra inteira (regex)
}

func (c condicaoDeForma) sql(expr string) string {
	if c.palavraInteira {
		return fmt.Sprintf(`%s ~ $%d`, expr, c.idx)
	}
	return fmt.Sprintf(`%s LIKE $%d`, expr, c.idx)
}

// Até três caracteres: curta demais para um LIKE frouxo.
func formaCurta(forma string) bool {
	return len(forma) <= 3
}

// Busca produtos nas duas tabelas do catálogo, ignorando nulos
func SearchProdutosCarros(db *sql.DB, search string, limit int) ([]map[string]interface{}, error) {
	detectarColunas(db)

	// Colunas que cada tabela expõe à busca textual.
	colunasCarros := `pc.pro_codigo, pc.pro_descricao, pc.referencia,
					pc.carro_1, pc.ano_1,
					pc.carro_2, pc.ano_2,
					pc.carro_3, pc.ano_3,
					pc.carro_4, pc.ano_4,
					pc.carro_5, pc.ano_5,
					pc.carro_6, pc.ano_6,
					pc.carro_7, pc.ano_7,
					pc.carro_8, pc.ano_8,
					pc.carro_9, pc.ano_9,
					pc.carro_10, pc.ano_10`
	if nomeCeltaEmCarros {
		colunasCarros += `, pc."nomeCelta"`
	}

	// Em `products` a descrição é a coluna `name`; `brand` entra porque o
	// comprador digita a marca junto com a peça.
	colunasProducts := `p.pro_codigo, p.name, p.brand`
	if nomeCeltaEmProduct {
		colunasProducts += `, p."nomeCelta"`
	}

	var params []interface{}
	paramIndex := 1

	// Sem termo de busca a consulta vira listagem: as condições passam a TRUE e
	// o UNION devolve o catálogo inteiro, já deduplicado.
	condCarros, condProducts := "TRUE", "TRUE"

	/*
	   Cada palavra digitada vira um GRUPO de formas (ver `abreviacoes.go`):
	   "parabrisa" procura P/BRISA, PARABRISA, PARA-BRISA e PARA BRISA, porque
	   o cadastro escreve de todos esses jeitos. Entre grupos é AND (todas as
	   palavras precisam casar); dentro do grupo é OR (qualquer grafia serve).

	   `contemPorGrupo[i]` guarda, para cada grupo, os índices dos parâmetros
	   `%forma%` — a ordenação lá embaixo reutiliza os mesmos, sem repetir.
	*/
	grupos := dicionario.Interpretar(search)
	contemPorGrupo := make([][]condicaoDeForma, len(grupos))

	// Registra o parâmetro de uma forma e devolve a condição que o usa.
	registrar := func(forma string, prefixo bool) condicaoDeForma {
		c := condicaoDeForma{idx: paramIndex, palavraInteira: formaCurta(forma)}
		switch {
		case c.palavraInteira && prefixo:
			params = append(params, `^`+regexp.QuoteMeta(forma)+`\M`)
		case c.palavraInteira:
			params = append(params, `\m`+regexp.QuoteMeta(forma)+`\M`)
		case prefixo:
			params = append(params, forma+"%")
		default:
			params = append(params, "%"+forma+"%")
		}
		paramIndex++
		return c
	}

	if len(grupos) > 0 {
		var filtrosCarros, filtrosProducts []string

		for g, grupo := range grupos {
			var ouCarros, ouProducts []string

			for _, forma := range grupo.Formas {
				// concat_ws ignora NULLs e converte qualquer tipo de coluna
				// para texto, evitando erro de cast no Postgres.
				//
				// Os dois lados do UNION consomem o MESMO $n — é a mesma forma.
				c := registrar(forma, false)
				ouCarros = append(ouCarros,
					c.sql(fmt.Sprintf(`UPPER(concat_ws(' ', %s))`, colunasCarros)))
				ouProducts = append(ouProducts,
					c.sql(fmt.Sprintf(`UPPER(concat_ws(' ', %s))`, colunasProducts)))
				contemPorGrupo[g] = append(contemPorGrupo[g], c)
			}

			// O pro_codigo aparece de novo, sozinho, pela palavra COMO FOI
			// DIGITADA (não pelas formas expandidas — código não se abrevia):
			// dentro do concat_ws ele fica colado no vizinho e casa pelo mesmo
			// LIKE frouxo que casa qualquer ano; testá-lo à parte deixa o
			// casamento por código explícito.
			ouCarros = append(ouCarros, fmt.Sprintf(
				`UPPER(COALESCE(CAST(pc.pro_codigo AS TEXT), '')) LIKE $%d`, paramIndex))
			ouProducts = append(ouProducts, fmt.Sprintf(
				`UPPER(COALESCE(CAST(p.pro_codigo AS TEXT), '')) LIKE $%d`, paramIndex))
			params = append(params, "%"+grupo.Original+"%")
			paramIndex++

			filtrosCarros = append(filtrosCarros, "("+strings.Join(ouCarros, " OR ")+")")
			filtrosProducts = append(filtrosProducts, "("+strings.Join(ouProducts, " OR ")+")")
		}

		condCarros = strings.Join(filtrosCarros, " AND ")
		condProducts = strings.Join(filtrosProducts, " AND ")
	}

	termoInteiro := strings.ToUpper(strings.TrimSpace(search))
	ordenacao := ordemPorCodigo

	switch {
	case termoInteiro == "":
		// Listagem: ordem por código, estável entre chamadas, para o portal
		// paginar sem linha trocando de página entre requisições.

	case apenasDigitos(termoInteiro):
		/*
		   Quem digita só números está digitando um código, não uma descrição.

		   Nenhuma descrição começa por "31500", então o ranking por descrição
		   empataria TODOS os resultados no último degrau e a ordem voltaria a
		   ser a que o Postgres entregasse — o próprio código procurado saindo
		   no meio da lista, atrás de peças que casaram só pelo ano.

		     0 — é exatamente o código digitado
		     1 — o código COMEÇA com o que foi digitado
		     2 — o código CONTÉM o que foi digitado
		     3 — casou por outra coluna
		*/
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
					WHEN UPPER(pro_codigo) = $%d THEN 0
					WHEN UPPER(pro_codigo) LIKE $%d THEN 1
					WHEN UPPER(pro_codigo) LIKE $%d THEN 2
					ELSE 3
				END,
				`+ordemPorCodigo, idxExato, idxPrefixo, idxContem)

	case len(grupos) == 0:
		// Só caracteres que a normalização descarta: nada para ranquear.

	default:
		/*
		   Busca textual: o que o comprador digitou vem primeiro, medido sobre a
		   DESCRIÇÃO, que é o que aparece na tela.
		     0 — a descrição COMEÇA pela primeira palavra e CONTÉM todas as
		         outras ("P/BRISA GOL G5" para "parabrisa gol")
		     1 — a descrição contém todas as palavras, mas não começa pela
		         primeira ("BORRACHA P/BRISA GOL")
		     2 — casou por outra coluna: código, referência, marca ou carro/ano

		   Medido pelas FORMAS de cada grupo, não pelo texto digitado: quem
		   digita "parabrisa gol" quer "P/BRISA GOL" no topo, e a descrição
		   nunca começa por "PARABRISA". Exige todas as palavras, não só a
		   primeira: ranquear pela primeira sozinha jogaria qualquer "P/BRISA"
		   para a frente de "P/BRISA GOL".
		*/
		descricao := `UPPER(COALESCE(pro_descricao, ''))`

		var comecaPrimeira []string
		for _, forma := range grupos[0].Formas {
			comecaPrimeira = append(comecaPrimeira, registrar(forma, true).sql(descricao))
		}

		// `%forma%` já está nos parâmetros do WHERE: reaproveita os índices.
		contemGrupo := func(g int) string {
			var ou []string
			for _, c := range contemPorGrupo[g] {
				ou = append(ou, c.sql(descricao))
			}
			return "(" + strings.Join(ou, " OR ") + ")"
		}

		contemTodas := make([]string, 0, len(grupos))
		for g := range grupos {
			contemTodas = append(contemTodas, contemGrupo(g))
		}
		contemDemais := []string{"TRUE"}
		if len(grupos) > 1 {
			contemDemais = contemTodas[1:]
		}

		ordenacao = fmt.Sprintf(`
				CASE
					WHEN (%s) AND %s THEN 0
					WHEN %s THEN 1
					ELSE 2
				END,
				pro_descricao ASC,
				`+ordemPorCodigo,
			strings.Join(comecaPrimeira, " OR "),
			strings.Join(contemDemais, " AND "),
			strings.Join(contemTodas, " AND "))
	}

	idxLimite := paramIndex
	params = append(params, limit)

	query := fmt.Sprintf(consultaUnificada, condCarros, condProducts, ordenacao, idxLimite)

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
