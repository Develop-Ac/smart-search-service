package main

import (
	"reflect"
	"strings"
	"testing"
)

// Recorte da planilha real, com as inconsistências que ela tem de verdade:
// o mesmo prefixo com três grafias do termo, erro de digitação, prefixo com
// dois sentidos.
func dicionarioDeTeste() *Dicionario {
	return NovoDicionario([][2]string{
		{"P/BRISA", "PARA-BRISA"},
		{"P/BRISA", "PARA BRISA"},
		{"P/BRISA", "PARA  BRISA"},
		{"P/BRISAS", "PARA-BRISA"},
		{"PARABRISA", "PARA-BRISA"},
		{"PARABRISAS", "PARA-BRISA"},
		{"PARA-BRISA", "PARA-BRISA"},
		{"LIMP.P/BRISA", "LIMPA PARA-BRISA"},
		{"P/CHOQUE", "PARA-CHOQUE"},
		{"PARACHOQUE", "PARA-CHOQUE"},
		{"P/LAMA", "PARALAMA"},
		{"PARALAMA", "PARALAMA"},
		{"DIANT", "DINATEIRO"}, // erro de digitação da planilha
		{"DIANT.", "DIANTEIRO"},
		{"DT", "DIANTEIRO"},
		{"DIANTEIRO", "DIANTEIRO"},
		{"TRAS", "TRASEIRO"},
		{"TRAS.", "TRASEIRO"},
		{"TRAZ", "TRASEIRO"},
		{"LE", "LADO ESQUERDO"},
		{"PTA", "PRETA"},
		{"PTA", "PORTA"},
		{"C/CONTR", "COM CONTR"},
		{"C/CONTR", "COM CONTROLE"},
		{"C/CONTROLE", "COM CONTROLE"},
		{"C/DESEMB", "COM DESEMBASSADOR"},
		{"C/DESEMB", "COM DESEMBA�ADOR"}, // acento perdido na codificação
		{"C/DESEMBACADOR", "COM DESEMBACADOR"},
		{"OSRAN / NARVA", "OSRAN / NARVA"},
	})
}

func chaves(grupos []Grupo) []string {
	if len(grupos) == 0 {
		return nil
	}
	saida := make([]string, len(grupos))
	for i, g := range grupos {
		if g.Chave != "" {
			saida[i] = g.Chave
		} else {
			saida[i] = "?" + g.Original
		}
	}
	return saida
}

func TestInterpretar(t *testing.T) {
	d := dicionarioDeTeste()

	casos := []struct {
		busca   string
		esperado []string
	}{
		// A abreviação do cadastro, como o vendedor digita.
		{"P/BRISA", []string{"PARABRISA"}},
		{"p/brisa gol", []string{"PARABRISA", "?GOL"}},
		// Por extenso, com e sem hífen, com acento, no plural.
		{"parabrisa", []string{"PARABRISA"}},
		{"Pára-Brisa", []string{"PARABRISA"}},
		{"para-brisas", []string{"PARABRISA"}},
		// Duas palavras que são um termo só.
		{"para brisa gol", []string{"PARABRISA", "?GOL"}},
		// Três palavras que são um termo só, antes de duas.
		{"limpa para brisa", []string{"LIMPAPARABRISA"}},
		// Palavra colada.
		{"parabrisagol", []string{"PARABRISA", "?GOL"}},
		// Plural colado fica no grupo.
		{"dianteiros", []string{"DIANTEIRO"}},
		// Abreviações coladas por ponto.
		{"diant.p/lama", []string{"DIANTEIRO", "PARALAMA"}},
		{"tras.le", []string{"TRASEIRO", "LADOESQUERDO"}},
		// Ponto sozinho não resolve nada conhecido: fica inteira.
		{"1.1/2X3/4", []string{"?1.1/2X3/4"}},
		// Fora do dicionário: como veio, normalizada.
		{"amortecedor", []string{"?AMORTECEDOR"}},
		{"", nil},
	}

	for _, c := range casos {
		got := chaves(d.Interpretar(c.busca))
		if !reflect.DeepEqual(got, c.esperado) {
			t.Errorf("Interpretar(%q) = %v, esperado %v", c.busca, got, c.esperado)
		}
	}
}

func TestFormasDoGrupo(t *testing.T) {
	d := dicionarioDeTeste()
	g := d.Interpretar("parabrisa")[0]

	// Todas as grafias do cadastro entram; "P/BRISAS" e "LIMP.P/BRISA" não,
	// porque `%P/BRISA%` já as alcança (poda).
	esperadas := []string{"P/BRISA", "PARABRISA", "PARA BRISA", "PARA-BRISA"}
	if !reflect.DeepEqual(g.Formas, esperadas) {
		t.Errorf("formas = %v, esperado %v", g.Formas, esperadas)
	}
	if g.Canonico != "PARA-BRISA" {
		t.Errorf("canonico = %q, esperado PARA-BRISA", g.Canonico)
	}
}

func TestErroDeDigitacaoDaPlanilhaNaoSeparaGrupo(t *testing.T) {
	d := dicionarioDeTeste()

	// "DIANT" → "DINATEIRO" e "DIANT." → "DIANTEIRO" são o mesmo grupo.
	a := d.Interpretar("diant")[0].Chave
	b := d.Interpretar("dianteiro")[0].Chave
	if a != b {
		t.Errorf("DIANT (%s) e DIANTEIRO (%s) deveriam ser o mesmo grupo", a, b)
	}
	if !contem(d.Interpretar("dianteiro")[0].Formas, "DIANT") {
		t.Errorf("formas de DIANTEIRO deveriam incluir DIANT: %v", d.Interpretar("dianteiro")[0].Formas)
	}

	// "C/CONTR" → COM CONTR / COM CONTROLE: um contém o outro.
	if d.Interpretar("c/contr")[0].Chave != d.Interpretar("c/controle")[0].Chave {
		t.Errorf("C/CONTR e C/CONTROLE deveriam ser o mesmo grupo")
	}

	// Acento perdido ("DESEMBA�ADOR") e grafia errada ("DESEMBASSADOR").
	if d.Interpretar("c/desemb")[0].Chave != d.Interpretar("c/desembacador")[0].Chave {
		t.Errorf("C/DESEMB e C/DESEMBACADOR deveriam ser o mesmo grupo")
	}
}

func TestPrefixoComDoisSentidosNaoFunde(t *testing.T) {
	d := dicionarioDeTeste()

	// "PTA" é PRETA e PORTA; a primeira linha manda, e PORTA continua à parte.
	if g := d.Interpretar("pta")[0]; g.Chave != "PRETA" {
		t.Errorf("PTA deveria resolver para PRETA, veio %s", g.Chave)
	}
	if contem(d.Interpretar("preta")[0].Formas, "PORTA") {
		t.Errorf("PRETA não pode procurar PORTA")
	}
	if contem(d.Interpretar("porta")[0].Formas, "PRETA") {
		t.Errorf("PORTA não pode procurar PRETA")
	}
}

func TestRepresentanteEhAChaveMaisFrequente(t *testing.T) {
	d := dicionarioDeTeste()
	// "DINATEIRO" (uma linha, erro de digitação) não pode virar o nome do
	// grupo só porque entrou primeiro no union-find.
	if g := d.Interpretar("diant")[0]; g.Chave != "DIANTEIRO" {
		t.Errorf("chave do grupo = %s, esperado DIANTEIRO", g.Chave)
	}
	if g := d.Interpretar("c/contr")[0]; g.Chave != "COMCONTROLE" {
		t.Errorf("chave do grupo = %s, esperado COMCONTROLE", g.Chave)
	}
}

func TestParecidasNaoFundeCompostoComAPrimeiraPalavra(t *testing.T) {
	d := NovoDicionario([][2]string{
		{"CROMO", "CROMO"},
		{"CROMO/PRETA", "CROMO / PRETA"},
	})
	if contem(d.Interpretar("cromo")[0].Formas, "CROMO/PRETA") {
		t.Errorf("CROMO não pode herdar as formas de CROMO/PRETA")
	}
	if g := d.Interpretar("cromo/preta")[0]; g.Chave != "CROMOPRETA" {
		t.Errorf("CROMO/PRETA deveria ser grupo próprio, veio %s", g.Chave)
	}
}

func TestCanonizar(t *testing.T) {
	d := dicionarioDeTeste()

	// Busca e descrição caem no mesmo texto — é o que o fuzzy compara.
	busca := d.Canonizar("para-brisa gol 2015")
	descricao := d.Canonizar("P/BRISA GOL 2015")
	if busca != descricao {
		t.Errorf("Canonizar: %q != %q", busca, descricao)
	}
	if busca != "PARABRISA GOL 2015" {
		t.Errorf("Canonizar = %q", busca)
	}
}

func TestPodar(t *testing.T) {
	got := podar([]string{"P/BRISAS", "P/BRISA", "LIMP.P/BRISA", "PARABRISA", "PARA-BRISA"})
	esperado := []string{"P/BRISA", "PARABRISA", "PARA-BRISA"}
	if !reflect.DeepEqual(got, esperado) {
		t.Errorf("podar = %v, esperado %v", got, esperado)
	}
}

func TestNormalizarTermo(t *testing.T) {
	if got := normalizarTermo("  Pára-Brisa   Gol "); got != "PARA-BRISA GOL" {
		t.Errorf("normalizarTermo = %q", got)
	}
	if got := compactar("PARA - BRISA"); got != "PARABRISA" {
		t.Errorf("compactar = %q", got)
	}
}

func TestParecidas(t *testing.T) {
	sim := [][2]string{
		{"DINATEIRO", "DIANTEIRO"},
		{"COMCONTR", "COMCONTROLE"},
		{"COMDESEMBASSADOR", "COMDESEMBACADOR"},
	}
	nao := [][2]string{
		{"PRETA", "PORTA"},
		{"GOL", "GOLF"},
		{"PARABRISA", "PARACHOQUE"},
	}
	for _, p := range sim {
		if !parecidas(p[0], p[1]) {
			t.Errorf("%s ~ %s deveriam ser parecidas", p[0], p[1])
		}
	}
	for _, p := range nao {
		if parecidas(p[0], p[1]) {
			t.Errorf("%s ~ %s NÃO deveriam ser parecidas", p[0], p[1])
		}
	}
}

// O dicionário embutido garante o mínimo mesmo sem a planilha.
func TestDicionarioEmbutido(t *testing.T) {
	d := NovoDicionario(abreviacoesEmbutidas)
	if g := d.Interpretar("parabrisa")[0]; !contem(g.Formas, "P/BRISA") {
		t.Errorf("embutido: parabrisa deveria procurar P/BRISA: %v", g.Formas)
	}
}

func contem(lista []string, item string) bool {
	for _, x := range lista {
		if strings.EqualFold(x, item) {
			return true
		}
	}
	return false
}
