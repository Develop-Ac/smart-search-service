package main

import (
	"database/sql"
	"fmt"
	"log"

	"github.com/xuri/excelize/v2"
)

func SeedProductsFromExcel(db *sql.DB, filePath string) error {
	// Abre arquivo Excel
	f, err := excelize.OpenFile(filePath)
	if err != nil {
		return fmt.Errorf("erro ao abrir arquivo Excel: %v", err)
	}
	defer f.Close()

	// Pega o nome da primeira planilha
	sheets := f.GetSheetList()
	if len(sheets) == 0 {
		return fmt.Errorf("nenhuma planilha encontrada")
	}

	sheetName := sheets[0]
	rows, err := f.GetRows(sheetName)
	if err != nil {
		return fmt.Errorf("erro ao ler planilha: %v", err)
	}

	if len(rows) < 2 {
		return fmt.Errorf("planilha vazia ou sem dados")
	}

	// Pula o header (primeira linha)
	for i := 1; i < len(rows); i++ {
		row := rows[i]
		
		// Esperando: PRO_CODIGO, PRO_DESCRICAO, ...
		if len(row) < 2 {
			continue
		}

		proCodigo := row[0]
		proDescricao := row[1]

		// Gera ID único baseado no código
		id := fmt.Sprintf("pro_%s", proCodigo)

		// Insere produto
		query := `
			INSERT OR IGNORE INTO products (id, pro_codigo, name, active)
			VALUES (?, ?, ?, 1)
		`

		_, err := db.Exec(query, id, proCodigo, proDescricao)
		if err != nil {
			log.Printf("Erro ao inserir produto %s: %v", proCodigo, err)
			continue
		}
	}

	totalRows := len(rows) - 1
	log.Printf("✓ %d produtos importados com sucesso!", totalRows)
	return nil
}
