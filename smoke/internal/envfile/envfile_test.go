package envfile

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadSkipsCommentsAndStripsQuotesAndTheEnvironmentWins(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".env")
	content := "# comment\n\nA=plain\nB = \"quoted\"\nC='single'\nD=has=equals\nnot a pair\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("A", "from-env")

	env, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	for k, want := range map[string]string{"A": "from-env", "B": "quoted", "C": "single", "D": "has=equals", "E": ""} {
		if got := env.Get(k); got != want {
			t.Errorf("Get(%s) = %q, want %q", k, got, want)
		}
	}
}

func TestLoadWithoutAPathReadsOnlyTheEnvironment(t *testing.T) {
	t.Setenv("ONLY_ENV", "x")
	env, err := Load("")
	if err != nil || env.Get("ONLY_ENV") != "x" {
		t.Errorf("env = %v, err = %v", env, err)
	}
}
