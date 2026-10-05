package main

import (
	"fmt"
	"log"
	"sort"
	"strings"
	"unicode"

	"github.com/xuri/excelize/v2"
)

/*
   Dicionário de abreviações do catálogo.

   O cadastro escreve "P/BRISA", "P/CHOQUE", "DIANT.", "C/DESEMB"; o comprador
   digita "parabrisa", "para-brisa", "para choque", "dianteiro". Sem
   tradução, cada grafia só encontra a si mesma. A planilha `Abreviações.xlsx`
   (colunas `Prefixo` → `Termo`) é a tabela de tradução mantida pela equipe.

   Cada linha da planilha vira um GRUPO: a chave do grupo é o termo por
   extenso "compactado" (só letras e dígitos: "PARA-BRISA" → "PARABRISA"), e
   todas as grafias que levam a ele — o prefixo abreviado, o termo por extenso,
   com hífen, com espaço — são as FORMAS do grupo. Na busca, cada palavra
   digitada é resolvida para o seu grupo e procurada no banco por TODAS as
   formas ("P/BRISA" OR "PARABRISA" OR "PARA-BRISA" OR "PARA BRISA").

   A planilha é inconsistente de propósito, porque é mantida à mão: o mesmo
   prefixo aparece três vezes com "PARA BRISA", "PARA-BRISA" e "PARA  BRISA".
   O compactar resolve a maior parte; o union-find abaixo funde o resto —
   grupos que compartilham um prefixo são o mesmo grupo.
*/

type Dicionario struct {
	// Grafia normalizada ("P/BRISA", "DIANT.") → chave do grupo.
	porPrefixo map[string]string
	// Grafia compactada ("PBRISA", "DIANT", "PARABRISAS") → chave do grupo.
	// Resolve o que vem sem pontuação ou com pontuação diferente da planilha.
	porCompacto map[string]string
	// Chave do grupo → formas a procurar no banco, já podadas (ver `podar`).
	formas map[string][]string
	// Chave do grupo → termo por extenso legível ("PARA-BRISA").
	canonico map[string]string
	// Chaves longas, da maior para a menor, para desgrudar palavra colada
	// ("PARABRISAGOL" → "PARABRISA" + "GOL").
	chavesLongas []string
}

// Grupo é uma palavra da busca já interpretada.
type Grupo struct {
	// Como foi digitada, normalizada ("P/BRISA", "PARA BRISA").
	Original string `json:"original"`
	// Chave do grupo reconhecido; vazia quando a palavra não está no dicionário.
	Chave string `json:"chave,omitempty"`
	// Termo por extenso ("PARA-BRISA"); vazio quando não reconhecida.
	Canonico string `json:"canonico,omitempty"`
	// O que procurar no banco. Sem dicionário, é a própria palavra.
	Formas []string `json:"formas"`
}

// Tamanho mínimo de chave para tentar desgrudar palavra colada. Abaixo disso
// "PORTAL" viraria "PORTA" + "L" e "GOLF" viraria "GOL" + "F".
const minChaveParaDesgrudar = 6

// Teto de formas por grupo no SQL: cada forma é um LIKE a mais em cada linha.
const maxFormasPorGrupo = 10

/*
   Fallback embutido: o que o serviço sabia antes da planilha existir. Garante
   que, sem o arquivo (caminho errado, imagem Docker antiga), "parabrisa"
   continue encontrando "P/BRISA" — nunca regride para a busca cega.
*/
var abreviacoesEmbutidas = [][2]string{
	{"P/BRISA", "PARA-BRISA"},
	{"P/BRISAS", "PARA-BRISA"},
	{"PARABRISA", "PARA-BRISA"},
	{"PARABRISAS", "PARA-BRISA"},
	{"PARA-BRISA", "PARA-BRISA"},
	{"PARA BRISA", "PARA-BRISA"},
	{"P/CHOQUE", "PARA-CHOQUE"},
	{"P/CHOQ", "PARA-CHOQUE"},
	{"PARACHOQUE", "PARA-CHOQUE"},
	{"PARA-CHOQUE", "PARA-CHOQUE"},
	{"P/LAMA", "PARALAMA"},
	{"PARALAMA", "PARALAMA"},
	{"DIANT", "DIANTEIRO"},
	{"DIANT.", "DIANTEIRO"},
	{"DIANTEIRO", "DIANTEIRO"},
	{"TRAS", "TRASEIRO"},
	{"TRAS.", "TRASEIRO"},
	{"TRAZ", "TRASEIRO"},
	{"TRASEIRO", "TRASEIRO"},
}

// O dicionário em uso. Carregado uma vez no arranque e só lido depois, então
// não precisa de trava.
var dicionario = NovoDicionario(abreviacoesEmbutidas)

// normalizarTermo põe em caixa alta, tira acento e espaço repetido.
// "Pára-Brisa  Gol" → "PARA-BRISA GOL".
func normalizarTermo(s string) string {
	s = removeAccents(strings.ToUpper(s))
	var b strings.Builder
	for _, r := range s {
		// U+FFFD é o que sobra de acento gravado com codificação errada na
		// planilha ("DESEMBA�ADOR"); fora, para a forma ainda casar o resto.
		if r == unicode.ReplacementChar {
			continue
		}
		b.WriteRune(r)
	}
	return strings.Join(strings.Fields(b.String()), " ")
}

// compactar deixa só letras e dígitos. "PARA-BRISA" = "PARA BRISA" =
// "PARABRISA" → "PARABRISA". É a identidade do grupo.
func compactar(s string) string {
	var b strings.Builder
	for _, r := range normalizarTermo(s) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// NovoDicionario monta o dicionário a partir de pares (prefixo, termo).
func NovoDicionario(pares [][2]string) *Dicionario {
	d := &Dicionario{
		porPrefixo:  map[string]string{},
		porCompacto: map[string]string{},
		formas:      map[string][]string{},
		canonico:    map[string]string{},
	}

	/*
	   Union-find sobre as chaves. "P/BRISA" aparece na planilha apontando
	   para "PARA BRISA" numa linha e "PARA-BRISA" noutra — compactadas são a
	   mesma chave. Mas "C/DESEMB" aponta para "COM DESEMBASSADOR" e para "COM
	   DESEMBAÇADOR" (chaves diferentes por erro de grafia): um prefixo que
	   leva a duas chaves diz que as duas são o mesmo grupo.
	*/
	pai := map[string]string{}
	var raiz func(string) string
	raiz = func(c string) string {
		if p, ok := pai[c]; ok && p != c {
			pai[c] = raiz(p)
			return pai[c]
		}
		if _, ok := pai[c]; !ok {
			pai[c] = c
		}
		return c
	}
	unir := func(a, b string) {
		ra, rb := raiz(a), raiz(b)
		if ra != rb {
			pai[rb] = ra
		}
	}

	type linha struct{ prefixo, compacto, termo, chave string }
	var linhas []linha
	// Chaves já vistas por grafia compacta do prefixo: "DIANT" e "DIANT."
	// são o mesmo prefixo, e a planilha lhes dá termos diferentes
	// ("DINATEIRO", "DIANTEIRO") — um erro de digitação, não dois sentidos.
	chavesDoPrefixo := map[string][]string{}

	for _, par := range pares {
		prefixo := normalizarTermo(par[0])
		termo := normalizarTermo(par[1])
		chave := compactar(termo)
		compacto := compactar(prefixo)
		if compacto == "" {
			compacto = prefixo
		}
		if prefixo == "" || termo == "" || chave == "" {
			continue
		}
		linhas = append(linhas, linha{prefixo, compacto, termo, chave})
		raiz(chave)

		/*
		   O mesmo prefixo levando a duas chaves só é o mesmo grupo quando as
		   chaves se PARECEM: "DINATEIRO" ~ "DIANTEIRO", "COMCONTR" ⊂
		   "COMCONTROLE". "PTA" → PRETA e "PTA" → PORTA são dois sentidos de
		   verdade, e fundi-los faria "preta" devolver porta.
		*/
		for _, outra := range chavesDoPrefixo[compacto] {
			if parecidas(outra, chave) {
				unir(outra, chave)
			}
		}
		chavesDoPrefixo[compacto] = append(chavesDoPrefixo[compacto], chave)
	}

	for _, l := range linhas {
		// Prefixo cuja forma compacta é a chave de OUTRO grupo ("AMARELA" como
		// prefixo de AMARELO, "AMARELA" como chave própria) une os dois.
		if _, existe := pai[l.compacto]; existe {
			unir(l.compacto, l.chave)
		}

		/*
		   Prefixo abreviado contido no começo de um prefixo maior: "C/DESEMB"
		   (→ COM DESEMBAÇADOR) e "C/DESEMBACADOR" (→ COM DESEMBACADOR) nunca
		   se encontrariam pela chave, porque a planilha grafou o termo de dois
		   jeitos. Se as chaves se parecem, é o mesmo grupo. Segunda passada,
		   para não depender da ordem das linhas.
		*/
		for k := 4; k < len(l.compacto); k++ {
			for _, outra := range chavesDoPrefixo[l.compacto[:k]] {
				if parecidas(outra, l.chave) {
					unir(outra, l.chave)
				}
			}
		}
	}

	/*
	   Representante de cada grupo: a chave mais frequente entre as linhas,
	   não a que por acaso virou raiz do union-find. "DIANT" → "DINATEIRO"
	   (uma linha, erro de digitação) uniu-se a "DIANTEIRO" (três linhas); a
	   chave do grupo tem que ser DIANTEIRO, que é o que aparece no
	   `/api/interpretar`, no fuzzy e na quebra de palavra colada.
	*/
	contagem := map[string]int{}
	var ordem []string
	for _, l := range linhas {
		if contagem[l.chave] == 0 {
			ordem = append(ordem, l.chave)
		}
		contagem[l.chave]++
	}
	representante := map[string]string{}
	for _, c := range ordem {
		r := raiz(c)
		if atual, ok := representante[r]; !ok || contagem[c] > contagem[atual] {
			representante[r] = c
		}
	}

	vistas := map[string]map[string]bool{}
	for _, l := range linhas {
		r := representante[raiz(l.chave)]
		if vistas[r] == nil {
			vistas[r] = map[string]bool{}
		}
		for _, forma := range []string{l.prefixo, l.termo} {
			if !vistas[r][forma] {
				vistas[r][forma] = true
				d.formas[r] = append(d.formas[r], forma)
			}
		}
		if _, ok := d.canonico[r]; !ok {
			d.canonico[r] = l.termo
		}
		// O primeiro a chegar fica: "PTA" é PRETA numa linha e PORTA noutra,
		// e a planilha está em ordem de importância.
		if _, ok := d.porPrefixo[l.prefixo]; !ok {
			d.porPrefixo[l.prefixo] = r
		}
		if _, ok := d.porCompacto[l.compacto]; !ok {
			d.porCompacto[l.compacto] = r
		}
		if _, ok := d.porCompacto[l.chave]; !ok {
			d.porCompacto[l.chave] = r
		}
	}

	for chave, formas := range d.formas {
		d.formas[chave] = podar(formas)
		if len(chave) >= minChaveParaDesgrudar {
			d.chavesLongas = append(d.chavesLongas, chave)
		}
	}
	sort.Slice(d.chavesLongas, func(i, j int) bool {
		if len(d.chavesLongas[i]) != len(d.chavesLongas[j]) {
			return len(d.chavesLongas[i]) > len(d.chavesLongas[j])
		}
		return d.chavesLongas[i] < d.chavesLongas[j]
	})

	return d
}

/*
   parecidas diz se duas chaves são a mesma palavra escrita de dois jeitos.

   Uma é a outra com um final curto a mais ("COMCONTR" ⊂ "COMCONTROLE",
   "DIANTEIRO" ⊂ "DIANTEIROS") ou, em palavra longa, a distância de edição é
   de até duas letras ("DINATEIRO" ~ "DIANTEIRO", "COMDESEMBASSADOR" ~
   "COMDESEMBACADOR").

   Os limites são o que separa isso de falso parente: "PRETA" ~ "PORTA"
   também estão a duas letras, mas têm cinco; "CROMO" ⊂ "CROMOPRETA", mas o
   que sobra ("PRETA") é outra palavra, não um sufixo.
*/
func parecidas(a, b string) bool {
	if a == b {
		return true
	}
	curta, longa := a, b
	if len(curta) > len(longa) {
		curta, longa = longa, curta
	}
	if len(curta) >= 4 && len(longa)-len(curta) <= 3 && strings.HasPrefix(longa, curta) {
		return true
	}
	return len(curta) >= 7 && levenshteinDistance(a, b) <= 2
}

/*
   podar tira as formas redundantes para o LIKE.

   `%P/BRISA%` já casa tudo que `%P/BRISAS%` e `%LIMP.P/BRISA%` casariam —
   uma forma que CONTÉM outra do mesmo grupo não acrescenta resultado nenhum,
   só mais um LIKE por linha. Ficam as mais curtas, em ordem de tamanho, até
   o teto.
*/
func podar(formas []string) []string {
	ordenadas := make([]string, len(formas))
	copy(ordenadas, formas)
	sort.Slice(ordenadas, func(i, j int) bool {
		if len(ordenadas[i]) != len(ordenadas[j]) {
			return len(ordenadas[i]) < len(ordenadas[j])
		}
		return ordenadas[i] < ordenadas[j]
	})

	var mantidas []string
	for _, f := range ordenadas {
		redundante := false
		for _, m := range mantidas {
			if strings.Contains(f, m) {
				redundante = true
				break
			}
		}
		if !redundante {
			mantidas = append(mantidas, f)
		}
		if len(mantidas) == maxFormasPorGrupo {
			break
		}
	}
	return mantidas
}

// CarregarAbreviacoes lê a planilha (colunas Prefixo, Termo) e monta o
// dicionário por cima do embutido — o embutido garante o mínimo mesmo que a
// planilha tenha perdido uma linha.
func CarregarAbreviacoes(caminho string) (*Dicionario, error) {
	f, err := excelize.OpenFile(caminho)
	if err != nil {
		return nil, fmt.Errorf("abrir %s: %w", caminho, err)
	}
	defer f.Close()

	planilhas := f.GetSheetList()
	if len(planilhas) == 0 {
		return nil, fmt.Errorf("%s: nenhuma planilha", caminho)
	}
	linhas, err := f.GetRows(planilhas[0])
	if err != nil {
		return nil, fmt.Errorf("ler %s: %w", caminho, err)
	}

	pares := make([][2]string, 0, len(linhas)+len(abreviacoesEmbutidas))
	pares = append(pares, abreviacoesEmbutidas...)

	lidas := 0
	for i, linha := range linhas {
		if len(linha) < 2 {
			continue
		}
		prefixo, termo := strings.TrimSpace(linha[0]), strings.TrimSpace(linha[1])
		if i == 0 && strings.EqualFold(prefixo, "prefixo") {
			continue // cabeçalho
		}
		if prefixo == "" || termo == "" {
			continue // linha sem tradução ("CORREÇÃO FEITA" na coluna de nota)
		}
		// Prefixo com espaço ("OSRAN / NARVA") nunca casa uma palavra só.
		if strings.ContainsAny(prefixo, " \t") {
			continue
		}
		pares = append(pares, [2]string{prefixo, termo})
		lidas++
	}

	d := NovoDicionario(pares)
	log.Printf("[ABREVIACOES] %s: %d linhas úteis, %d grupos", caminho, lidas, len(d.formas))
	return d, nil
}

// resolver devolve a chave do grupo de UMA palavra, se houver.
func (d *Dicionario) resolver(palavra string) (string, bool) {
	p := normalizarTermo(palavra)
	if c, ok := d.porPrefixo[p]; ok {
		return c, true
	}
	if c, ok := d.porCompacto[compactar(p)]; ok {
		return c, true
	}
	return "", false
}

func (d *Dicionario) grupo(original, chave string) Grupo {
	formas := make([]string, 0, len(d.formas[chave])+1)
	formas = append(formas, d.formas[chave]...)
	/*
	   A palavra como foi digitada entra SEMPRE como forma.

	   A planilha tem linhas que só arrumam espaçamento: "/GOL" → "/ GOL",
	   "G5/G6" → "G5 / G6". Compactadas, viram a chave "GOL" — e quem digita
	   "gol" cai nesse grupo, cujas formas são só "/GOL" e "/ GOL". Sem esta
	   linha, "parabrisa gol" procurava "/GOL" e nunca achava "P/BRISA GOL".
	   A poda cuida do resto: "GOL" é mais curta e engole as outras duas.
	*/
	formas = podar(append(formas, normalizarTermo(original)))
	return Grupo{
		Original: original,
		Chave:    chave,
		Canonico: d.canonico[chave],
		Formas:   formas,
	}
}

func grupoLivre(palavra string) Grupo {
	return Grupo{Original: palavra, Formas: []string{palavra}}
}

/*
   Interpretar quebra a busca em palavras e resolve cada uma para o seu grupo.

     "p/brisa gol"       → [PARA-BRISA] [GOL]
     "para brisa gol"    → [PARA-BRISA] [GOL]       (duas palavras, um grupo)
     "parabrisagol"      → [PARA-BRISA] [GOL]       (palavra colada)
     "diant.p/lama"      → [DIANTEIRO] [PARALAMA]   (abreviações coladas por ponto)
     "amortecedor"       → [AMORTECEDOR]            (fora do dicionário: como veio)
*/
func (d *Dicionario) Interpretar(busca string) []Grupo {
	palavras := strings.Fields(normalizarTermo(busca))
	var grupos []Grupo

	for i := 0; i < len(palavras); {
		// Vizinhas que, juntas, formam um grupo: "PARA" + "BRISA". Tenta três
		// antes de duas para "LIMPA PARA BRISA" não virar LIMPA + PARA-BRISA.
		juntou := false
		for n := 3; n >= 2; n-- {
			if i+n > len(palavras) {
				continue
			}
			juntas := strings.Join(palavras[i:i+n], " ")
			if c, ok := d.porCompacto[compactar(juntas)]; ok {
				grupos = append(grupos, d.grupo(juntas, c))
				i += n
				juntou = true
				break
			}
		}
		if juntou {
			continue
		}
		grupos = append(grupos, d.expandirPalavra(palavras[i], 0)...)
		i++
	}
	return grupos
}

func (d *Dicionario) expandirPalavra(palavra string, profundidade int) []Grupo {
	if c, ok := d.resolver(palavra); ok {
		return []Grupo{d.grupo(palavra, c)}
	}
	if profundidade > 3 {
		return []Grupo{grupoLivre(palavra)}
	}

	/*
	   Abreviações coladas por ponto: "DIANT.P/LAMA", "TRAS.LE". Só vale a
	   pena quebrar se ao menos um pedaço for conhecido — "1.1/2X3/4" é uma
	   medida, e partida em "1" + "1/2X3/4" só traria lixo.
	*/
	if strings.Contains(palavra, ".") {
		pedacos := strings.FieldsFunc(palavra, func(r rune) bool { return r == '.' })
		if len(pedacos) >= 2 {
			conhecido := false
			for _, p := range pedacos {
				if _, ok := d.resolver(p); ok {
					conhecido = true
					break
				}
			}
			if conhecido {
				var grupos []Grupo
				for _, p := range pedacos {
					grupos = append(grupos, d.expandirPalavra(p, profundidade+1)...)
				}
				return grupos
			}
		}
	}

	/*
	   Palavra colada: "PARABRISAGOL" começa com a chave "PARABRISA". Só para
	   chave longa e resto com ao menos duas letras; resto de um "S" é plural
	   ("DIANTEIROS") e fica no grupo.
	*/
	compacta := compactar(palavra)
	for _, chave := range d.chavesLongas {
		if !strings.HasPrefix(compacta, chave) || len(compacta) == len(chave) {
			continue
		}
		resto := compacta[len(chave):]
		if resto == "S" {
			return []Grupo{d.grupo(palavra, chave)}
		}
		if len(resto) >= 2 {
			grupos := []Grupo{d.grupo(chave, chave)}
			return append(grupos, d.expandirPalavra(resto, profundidade+1)...)
		}
	}

	return []Grupo{grupoLivre(palavra)}
}

/*
   Canonizar devolve o texto no vocabulário do dicionário: cada palavra
   reconhecida vira a chave do grupo. "P/BRISA GOL" e "para-brisa gol" viram
   os mesmos "PARABRISA GOL" — é o que permite o fuzzy comparar a busca com a
   descrição sem que a abreviação conte como erro de digitação.
*/
func (d *Dicionario) Canonizar(texto string) string {
	grupos := d.Interpretar(texto)
	partes := make([]string, 0, len(grupos))
	for _, g := range grupos {
		if g.Chave != "" {
			partes = append(partes, g.Chave)
		} else {
			partes = append(partes, g.Original)
		}
	}
	return strings.Join(partes, " ")
}
