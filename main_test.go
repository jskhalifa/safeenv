package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSafeenvPayload(t *testing.T) {
	for _, tc := range []struct {
		name string
		in   string
		want string
	}{
		{"safeenv", safeenvHeader + "age-body", "age-body"},
		{"plain-age", "age-body", "age-body"},
	} {
		if got := string(safeenvPayload([]byte(tc.in))); got != tc.want {
			t.Fatalf("%s: got %q", tc.name, got)
		}
	}
}

func TestIdentityFileAcceptsPath(t *testing.T) {
	got, cleanup, err := identityFile("secrets.key")
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()
	if got != "secrets.key" {
		t.Fatalf("got %q", got)
	}
}

func TestPullArgs(t *testing.T) {
	host, folder, key, err := pullArgs([]string{"user@ip", "/app", "--decrypt=secret.key"})
	if err != nil {
		t.Fatal(err)
	}
	if host != "user@ip" || folder != "/app" || key != "secret.key" {
		t.Fatalf("got %q %q %q", host, folder, key)
	}
	if _, _, _, err := pullArgs([]string{"user@ip", "/app", "--wat"}); err == nil {
		t.Fatal("expected bad flag error")
	}
}

func TestSafeenvPathKeepsDotfileName(t *testing.T) {
	if got := safeenvPath(".env.cloud"); got != ".env.cloud.safeenv" {
		t.Fatalf("got %q", got)
	}
}

func TestParseEnv(t *testing.T) {
	got, err := parseEnv([]byte(`
# nope
FOO=bar
EMPTY=
QUOTED="bar baz"
SINGLE='zap'
export PORT=8080
`))
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"FOO=bar", "EMPTY=", "QUOTED=bar baz", "SINGLE=zap", "PORT=8080"}
	if len(got) != len(want) {
		t.Fatalf("got %v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v", got)
		}
	}
}

func TestAppendGitignore(t *testing.T) {
	dir := t.TempDir()
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(old)
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(".gitignore", []byte(".env\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := appendGitignore([]string{".env", ".env.safeenv"}); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(".gitignore")
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != ".env\n.env.safeenv\n" {
		t.Fatalf("got %q", got)
	}
}

func TestCleanOnlySafeenvFiles(t *testing.T) {
	dir := t.TempDir()
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(old)
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{".env", ".env.safeenv.1", ".env.safeenv.2"} {
		if err := os.WriteFile(name, []byte("x"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := clean(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(".env"); err != nil {
		t.Fatal(".env should stay:", err)
	}
	matches, err := filepath.Glob(".env.safeenv.*")
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 0 {
		t.Fatalf("leftovers: %v", matches)
	}
}
