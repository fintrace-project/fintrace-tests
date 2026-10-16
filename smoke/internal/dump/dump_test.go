package dump

import (
	"bytes"
	"database/sql"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadBuildsAQueryableSQLiteFileAndReadsTheExpectedState(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "dump.sql", `-- a comment line
CREATE TABLE Chronicle(uid INTEGER PRIMARY KEY AUTOINCREMENT, badge INTEGER, type INTEGER, params TEXT);

INSERT INTO Chronicle VALUES (1, 7, 1, '{"version":3}');
INSERT INTO Chronicle VALUES (2, 8, 20, '{"uid":1,"name":"Cash; with a semicolon","money":150,"currency":978}');
`)
	write(t, dir, "expected.json", `{"size":"S","seed":42,"rows":2,
		"accounts":[{"uid":1,"name":"Cash","currency":"EUR","archived":false,"balance":150.00}],
		"categories":0,"operations":0,"transfers":0,"diagnostics":0}`)

	d, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(d.SQLite, []byte("SQLite format 3\x00")) {
		t.Fatalf("not a SQLite file: % x", d.SQLite[:16])
	}
	if got := rows(t, d.SQLite); got != 2 {
		t.Errorf("Chronicle rows = %d, want 2", got)
	}
	want := Account{Name: "Cash", Currency: "EUR", Balance: 150}
	if d.Expected.Seed != 42 || len(d.Expected.Accounts) != 1 || d.Expected.Accounts[0] != want {
		t.Errorf("expected = %+v", d.Expected)
	}
}

func TestTheCommittedSDumpLoadsWithTheRowsItDeclares(t *testing.T) {
	d, err := Load(filepath.Join("..", "..", "..", "dumps", "mok", "S"))
	if err != nil {
		t.Fatal(err)
	}
	if got := rows(t, d.SQLite); got != d.Expected.Rows {
		t.Errorf("Chronicle rows = %d, expected.json declares %d", got, d.Expected.Rows)
	}
}

func TestLoadNamesTheLineThatFails(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "dump.sql", "CREATE TABLE t(a INTEGER);\nINSERT INTO missing VALUES (1);\n")
	write(t, dir, "expected.json", `{}`)

	if _, err := Load(dir); err == nil || !bytes.Contains([]byte(err.Error()), []byte("line 2")) {
		t.Errorf("err = %v, want it to name line 2", err)
	}
}

func write(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func rows(t *testing.T, file []byte) int {
	t.Helper()
	path := filepath.Join(t.TempDir(), "check.sqlite")
	if err := os.WriteFile(path, file, 0o600); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var n int
	if err := db.QueryRow("SELECT count(*) FROM Chronicle").Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}
