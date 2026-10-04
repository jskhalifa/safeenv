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

func TestMapTarget(t *testing.T) {
	goos, goarch := mapTarget("linux", "x86_64")
	if goos != "linux" || goarch != "amd64" {
		t.Fatalf("got %s/%s", goos, goarch)
	}
	goos, goarch = mapTarget("linux", "aarch64")
	if goos != "linux" || goarch != "arm64" {
		t.Fatalf("got %s/%s", goos, goarch)
	}
}

func TestReleaseURL(t *testing.T) {
	got := releaseURL("linux", "amd64")
	want := "https://github.com/jskhalifa/safeenv/releases/download/0.1.1/safeenv-linux-amd64"
	if got != want {
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

func TestPrivateKeyUsesCustomEnvName(t *testing.T) {
	t.Setenv("MY_SAFEENV_KEY", "secret.key")
	got, err := privateKey("", "MY_SAFEENV_KEY")
	if err != nil {
		t.Fatal(err)
	}
	if got != "secret.key" {
		t.Fatalf("got %q", got)
	}
}

func TestKeyStatus(t *testing.T) {
	dir := t.TempDir()
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(old)
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(".env.cloud.pub", []byte("pub"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(".env.cloud.key", []byte("key"), 0600); err != nil {
		t.Fatal(err)
	}
	if got := keyStatus(".env.cloud.safeenv"); got != "public-private" {
		t.Fatalf("got %q", got)
	}
}

func TestPushRejectsPlaintextEnv(t *testing.T) {
	if err := push(".env.cloud", "user@ip", "/app"); err == nil {
		t.Fatal("expected plaintext push error")
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
