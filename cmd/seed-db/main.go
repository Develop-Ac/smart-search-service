// +build ignore

package main

import (
	"database/sql"
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/joho/godotenv"
	"github.com/xuri/excelize/v2"
	_ "modernc.org/sqlite"
)

type Product struct {
	ID        string
	ProCodigo string
	Name      string
	Brand     string
	Active    bool
}

func initDB(dbPath string) (*sql.DB, error) {
	dsn := fmt.Sprintf("file:%s", dbPath)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}

	if err := db.Ping(); err != nil {
		return nil, err
	}

	createTableQuery := `
	CREATE TABLE IF NOT EXISTS products (
		id TEXT PRIMARY KEY,
		pro_codigo TEXT NOT NULL,
		name TEXT NOT NULL,
		category TEXT,
		brand TEXT,
		active INTEGER DEFAULT 1
	);
	`

	if _, err := db.Exec(createTableQuery); err != nil {
		return nil, fmt.Errorf("erro ao criar tabela: %v", err)
	}

	return db, nil
}

func seedFromExcel(db *sql.DB, filePath string) error {
	f, err := excelize.OpenFile(filePath)
	if err != nil {
		return fmt.Errorf("erro ao abrir arquivo: %v", err)
	}
	defer f.Close()

	sheets := f.GetSheetList()
	if len(sheets) == 0 {
		return fmt.Errorf("nenhuma planilha encontrada")
	}

	rows, err := f.GetRows(sheets[0])
	if err != nil {
		return fmt.Errorf("erro ao ler planilha: %v", err)
	}

	if len(rows) < 2 {
		return fmt.Errorf("planilha vazia")
	}

	tx, err := db.Begin()
	if err != nil {
		return err
	}

	inserted := 0
	for i := 1; i < len(rows); i++ {
		row := rows[i]
		if len(row) < 2 {
			continue
		}

		proCodigo := row[0]
		proDescricao := row[1]

		id := fmt.Sprintf("pro_%s", proCodigo)

		_, err := tx.Exec(
			`INSERT OR IGNORE INTO products (id, pro_codigo, name, active)
			 VALUES (?, ?, ?, 1)`,
			id, proCodigo, proDescricao,
		)
		if err != nil {
			log.Printf("Erro ao inserir %s: %v", proCodigo, err)
			continue
		}
		inserted++
	}

	if err := tx.Commit(); err != nil {
		return err
	}

	log.Printf("✓ %d produtos importados com sucesso!", inserted)
	return nil
}

func main() {
	excelFile := flag.String("file", "", "Caminho do arquivo Excel")
	flag.Parse()

	godotenv.Load()

	if *excelFile == "" {
		log.Fatal("Use: go run seed-db.go -file=produtos_limpos.xlsx")
	}

	dbPath := os.Getenv("DATABASE_URL")
	if dbPath == "" {
		dbPath = "dev.db"
	}

	db, err := initDB(dbPath)
	if err != nil {
		log.Fatalf("Erro ao conectar: %v", err)
	}
	defer db.Close()

	if err := seedFromExcel(db, *excelFile); err != nil {
		log.Fatalf("Erro ao importar: %v", err)
	}
}
