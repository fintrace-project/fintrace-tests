package envfile

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

// Env is configuration read from a KEY=VALUE file, with the process environment taking precedence.
type Env map[string]string

func Load(path string) (Env, error) {
	env := Env{}
	if path == "" {
		return env, nil
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open env file: %w", err)
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		env[strings.TrimSpace(k)] = strings.Trim(strings.TrimSpace(v), `"'`)
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("read env file: %w", err)
	}
	return env, nil
}

func (e Env) Get(key string) string {
	if v, ok := os.LookupEnv(key); ok {
		return v
	}
	return e[key]
}
