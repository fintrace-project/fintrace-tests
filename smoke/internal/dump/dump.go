// Package dump reads a synthetic dump kept under dumps/<format>/<size>/: dump.sql and expected.json,
// written together by one run.
package dump

import (
	"bufio"
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	_ "modernc.org/sqlite"
)

// Expected is the state Core should end in after importing the dump; its shape is the same for every format.
type Expected struct {
	Size        string    `json:"size"`
	Seed        int64     `json:"seed"`
	Rows        int       `json:"rows"`
	Accounts    []Account `json:"accounts"`
	Categories  int       `json:"categories"`
	Operations  int       `json:"operations"`
	Transfers   int       `json:"transfers"`
	Diagnostics int       `json:"diagnostics"`
}

type Account struct {
	Name     string  `json:"name"`
	Currency string  `json:"currency"`
	Archived bool    `json:"archived"`
	Balance  float64 `json:"balance"`
}

// Dump is a SQLite file's bytes, as an importer receives it, and what importing it should produce.
type Dump struct {
	SQLite   []byte
	Expected Expected
}

// Load builds the SQLite file from dir's dump.sql and reads dir's expected.json.
func Load(dir string) (Dump, error) {
	raw, err := os.ReadFile(filepath.Join(dir, "expected.json"))
	if err != nil {
		return Dump{}, err
	}
	var expected Expected
	if err := json.Unmarshal(raw, &expected); err != nil {
		return Dump{}, fmt.Errorf("read %s/expected.json: %w", dir, err)
	}
	script, err := os.ReadFile(filepath.Join(dir, "dump.sql"))
	if err != nil {
		return Dump{}, err
	}
	file, err := buildSQLite(script)
	if err != nil {
		return Dump{}, fmt.Errorf("build %s/dump.sql: %w", dir, err)
	}
	return Dump{SQLite: file, Expected: expected}, nil
}

// buildSQLite runs the script one line per statement, as the generator writes it, into a scratch file.
func buildSQLite(script []byte) ([]byte, error) {
	dir, err := os.MkdirTemp("", "fintrace-dump-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)
	path := filepath.Join(dir, "dump.sqlite")

	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	if err := run(db, script); err != nil {
		db.Close()
		return nil, err
	}
	if err := db.Close(); err != nil {
		return nil, err
	}
	return os.ReadFile(path)
}

func run(db *sql.DB, script []byte) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	lines := bufio.NewScanner(bytes.NewReader(script))
	lines.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for n := 1; lines.Scan(); n++ {
		line := strings.TrimSpace(lines.Text())
		if line == "" || strings.HasPrefix(line, "--") {
			continue
		}
		if _, err := tx.Exec(line); err != nil {
			tx.Rollback()
			return fmt.Errorf("line %d: %w", n, err)
		}
	}
	if err := lines.Err(); err != nil {
		tx.Rollback()
		return err
	}
	return tx.Commit()
}
