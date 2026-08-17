package main

import (
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

type SearchResult struct {
	ID                string  `json:"id"`
	Name              string  `json:"name"`
	ProCodigo         string  `json:"proCodigo"`
	Brand             string  `json:"brand,omitempty"`
	Score             float64 `json:"score"`
	StartsWithQuery   bool    `json:"startsWithQuery"`
	ContainsQuery     bool    `json:"containsQuery"`
}

const MINIMUM_SCORE_THRESHOLD = 20

// Normaliza texto para comparação
func normalize(text string) string {
	if text == "" {
		return ""
	}

	// Converte para maiúscula
	normalized := strings.ToUpper(text)

	// Normalizações específicas para português
	normalized = strings.ReplaceAll(normalized, "PARA BRISA", "P/BRISA")
	normalized = strings.ReplaceAll(normalized, "PARA-BRISA", "P/BRISA")
	normalized = strings.ReplaceAll(normalized, "PARABRISA", "P/BRISA")

	// Remove espaços múltiplos
	normalized = strings.Join(strings.Fields(normalized), " ")

	// Remove acentos
	normalized = removeAccents(normalized)

	return strings.TrimSpace(normalized)
}

// Remove acentos de uma string
func removeAccents(text string) string {
	// Normaliza para NFD (decomposição)
	t := norm.NFD.String(text)
	
	result := strings.Builder{}
	for _, r := range t {
		// Pula marcas diacríticas (acentos)
		if !unicode.Is(unicode.Mn, r) {
			result.WriteRune(r)
		}
	}

	return result.String()
}

// Calcula distância de Levenshtein
func levenshteinDistance(s1, s2 string) int {
	if len(s1) < len(s2) {
		return levenshteinDistance(s2, s1)
	}

	if len(s2) == 0 {
		return len(s1)
	}

	previous := make([]int, len(s2)+1)
	for i := 0; i <= len(s2); i++ {
		previous[i] = i
	}

	for i := 1; i <= len(s1); i++ {
		current := make([]int, len(s2)+1)
		current[0] = i

		for j := 1; j <= len(s2); j++ {
			insertions := previous[j] + 1
			deletions := current[j-1] + 1
			substitutions := previous[j-1]
			if s1[i-1] != s2[j-1] {
				substitutions += 1
			}

			current[j] = min(insertions, min(deletions, substitutions))
		}
		previous = current
	}

	return previous[len(s2)]
}

// Calcula similaridade entre duas strings (0-100)
func calculateSimilarity(str1, str2 string) float64 {
	longer := str1
	shorter := str2

	if len(str1) < len(str2) {
		longer = str2
		shorter = str1
	}

	if len(longer) == 0 {
		return 100
	}

	editDistance := levenshteinDistance(longer, shorter)
	return float64(len(longer)-editDistance) / float64(len(longer)) * 100
}

// Calcula token set ratio
func tokenSetRatio(query, text string) float64 {
	queryTokens := strings.Fields(query)
	textTokens := strings.Fields(text)

	allTokensPresent := true
	for _, qToken := range queryTokens {
		found := false
		for _, tToken := range textTokens {
			if strings.Contains(tToken, qToken) || strings.Contains(qToken, tToken) {
				found = true
				break
			}
		}
		if !found {
			allTokensPresent = false
			break
		}
	}

	if !allTokensPresent {
		return calculateSimilarity(query, text)
	}

	similarity := 0.0
	for _, qToken := range queryTokens {
		for _, tToken := range textTokens {
			sim := calculateSimilarity(qToken, tToken)
			if sim > similarity {
				similarity = sim
			}
		}
	}

	return similarity
}

// Calcula token sort ratio
func tokenSortRatio(query, text string) float64 {
	queryTokens := strings.Fields(query)
	textTokens := strings.Fields(text)

	// Ordena tokens (simples ordenação alfabética)
	sortTokens := func(tokens []string) []string {
		sorted := make([]string, len(tokens))
		copy(sorted, tokens)
		// Simples bubble sort para ordenação
		for i := 0; i < len(sorted); i++ {
			for j := i + 1; j < len(sorted); j++ {
				if sorted[j] < sorted[i] {
					sorted[i], sorted[j] = sorted[j], sorted[i]
				}
			}
		}
		return sorted
	}

	sortedQuery := strings.Join(sortTokens(queryTokens), " ")
	sortedText := strings.Join(sortTokens(textTokens), " ")

	return calculateSimilarity(sortedQuery, sortedText)
}

// Score final da busca (similar ao código TypeScript)
func calculateSearchScore(query, name, proCodigo string) float64 {
	normalizedQuery := normalize(query)
	normalizedName := normalize(name)
	normalizedCode := normalize(proCodigo)

	// Se a query está vazia
	if len(normalizedQuery) == 0 {
		return 0
	}

	score := 0.0

	// Verifica se o nome começa com a query
	if strings.HasPrefix(normalizedName, normalizedQuery) {
		score = 100
	} else if strings.Contains(normalizedName, normalizedQuery) {
		// Contém a query exatamente
		score = 90 + (float64(len(normalizedQuery)) / float64(len(normalizedName)) * 10)
	} else {
		// Busca multi-palavra: verifica quantos tokens da query aparecem no nome
		queryTokens := strings.Fields(normalizedQuery)
		nameTokens := strings.Fields(normalizedName)
		
		matchedTokens := 0
		totalMatches := 0
		
		for _, qToken := range queryTokens {
			for _, nToken := range nameTokens {
				if strings.Contains(nToken, qToken) || strings.Contains(qToken, nToken) {
					matchedTokens++
					totalMatches++
					break
				}
			}
		}
		
		// Se encontrou pelo menos 1 token, calcular score
		if matchedTokens > 0 {
			// Percentual de tokens encontrados
			tokenMatchRatio := float64(matchedTokens) / float64(len(queryTokens)) * 100
			
			// Aplica fuzzy matching para refinar
			setSimilarity := tokenSetRatio(normalizedQuery, normalizedName)
			sortSimilarity := tokenSortRatio(normalizedQuery, normalizedName)
			
			// Se todos os tokens foram encontrados, score mais alto
			if matchedTokens == len(queryTokens) {
				fuzzyScore := (setSimilarity + sortSimilarity) / 2
				score = (tokenMatchRatio + fuzzyScore) / 2
			} else {
				// Encontrou apenas alguns tokens, mas ainda relevante
				fuzzyScore := (setSimilarity + sortSimilarity) / 2
				score = (tokenMatchRatio * 0.7) + (fuzzyScore * 0.3)
			}
		} else {
			// Nenhum token encontrado, usa fuzzy matching puro
			setSimilarity := tokenSetRatio(normalizedQuery, normalizedName)
			sortSimilarity := tokenSortRatio(normalizedQuery, normalizedName)
			directSimilarity := calculateSimilarity(normalizedQuery, normalizedName)

			score = (setSimilarity + sortSimilarity + directSimilarity) / 3
		}
	}

	// Bonus para código do produto
	if strings.Contains(normalizedCode, normalizedQuery) {
		score = (score + 100) / 2
	}

	return score
}

// Helper function para encontrar o mínimo
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
