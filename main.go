package main

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

const safeenvHeader = "SAFEENV AGE V1\n"

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "safeenv:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		return usage()
	}
	switch args[0] {
	case "init":
		if len(args) != 2 {
			return usage()
		}
		return initEnv(args[1])
	case "key-gen":
		name := "secrets"
		if len(args) > 2 {
			return usage()
		}
		if len(args) == 2 {
			name = args[1]
		}
		return keyGen(name)
	case "encrypt":
		if len(args) != 2 {
			return usage()
		}
		return encrypt(args[1])
	case "push":
		if len(args) != 3 {
			return usage()
		}
		return push(args[1], args[2])
	case "pull":
		host, folder, key, err := pullArgs(args[1:])
		if err != nil {
			return err
		}
		return pull(host, folder, key)
	case "clean":
		if len(args) != 1 {
			return usage()
		}
		return clean()
	case "decrypt":
		if len(args) > 2 {
			return usage()
		}
		path := os.Getenv("ENV_SECRET_FILE")
		if len(args) == 2 {
			path = args[1]
		}
		if path == "" {
			return usage()
		}
		return decrypt(path)
	default:
		return runWithEnv(args)
	}
}

func usage() error {
	return errors.New("usage: safeenv <file.safeenv> -- <command> [args...] | init <file.env> | key-gen [name] | encrypt <file.env> | push <user>@<ip> <folder> | pull <user>@<ip> <folder> [--decrypt=<secret.key>] | decrypt [file.safeenv] | clean")
}

func initEnv(path string) error {
	if err := os.WriteFile(path, nil, 0600); err != nil {
		return err
	}
	return appendGitignore([]string{
		path,
		safeenvPath(path),
		path + ".key",
		path + ".pub",
		".env.safeenv.*",
		".env.decrypted",
		"safeenv",
	})
}

func appendGitignore(lines []string) error {
	if _, err := os.Stat(".gitignore"); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	src, err := os.ReadFile(".gitignore")
	if err != nil {
		return err
	}
	seen := map[string]bool{}
	for _, line := range strings.Split(string(src), "\n") {
		seen[strings.TrimSpace(line)] = true
	}
	var b strings.Builder
	for _, line := range lines {
		if line != "" && !seen[line] {
			b.WriteString(line)
			b.WriteByte('\n')
		}
	}
	if b.Len() == 0 {
		return nil
	}
	f, err := os.OpenFile(".gitignore", os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	if len(src) > 0 && !bytes.HasSuffix(src, []byte("\n")) {
		if _, err := f.WriteString("\n"); err != nil {
			return err
		}
	}
	_, err = f.WriteString(b.String())
	return err
}

func ensureAge() error {
	if _, err := exec.LookPath("age"); err == nil {
		if _, err := exec.LookPath("age-keygen"); err == nil {
			return nil
		}
	}
	for _, cmd := range [][]string{
		{"brew", "install", "age"},
		{"apt-get", "update"},
		{"apt-get", "install", "-y", "age"},
		{"sudo", "apt-get", "update"},
		{"sudo", "apt-get", "install", "-y", "age"},
		{"apk", "add", "age"},
		{"sudo", "apk", "add", "age"},
		{"dnf", "install", "-y", "age"},
		{"sudo", "dnf", "install", "-y", "age"},
		{"yum", "install", "-y", "age"},
		{"sudo", "yum", "install", "-y", "age"},
		{"pacman", "-Sy", "--noconfirm", "age"},
		{"sudo", "pacman", "-Sy", "--noconfirm", "age"},
	} {
		if _, err := exec.LookPath(cmd[0]); err != nil {
			continue
		}
		c := exec.Command(cmd[0], cmd[1:]...)
		c.Stdout, c.Stderr = os.Stdout, os.Stderr
		_ = c.Run()
		if _, err := exec.LookPath("age"); err == nil {
			if _, err := exec.LookPath("age-keygen"); err == nil {
				return nil
			}
		}
	}
	return errors.New("age is missing; install it first: brew install age (macOS) or apt install age (Linux)")
}

func keyGen(name string) error {
	if err := ensureAge(); err != nil {
		return err
	}
	key, pub := name+".key", name+".pub"
	if err := command("age-keygen", "-o", key).Run(); err != nil {
		return err
	}
	out, err := exec.Command("age-keygen", "-y", key).Output()
	if err != nil {
		return err
	}
	if err := os.WriteFile(pub, out, 0644); err != nil {
		return err
	}
	return os.Chmod(key, 0600)
}

func encrypt(path string) error {
	if err := ensureAge(); err != nil {
		return err
	}
	out, err := exec.Command("age", "-R", path+".pub", path).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w: %s", err, out)
	}
	return os.WriteFile(safeenvPath(path), append([]byte(safeenvHeader), out...), 0600)
}

func push(host, folder string) error {
	bin, err := safeenvBinary()
	if err != nil {
		return err
	}
	files, err := filepath.Glob("*.safeenv")
	if err != nil {
		return err
	}
	if len(files) != 1 {
		return errors.New("push needs exactly one *.safeenv in current directory")
	}
	if err := command("ssh", host, "mkdir -p "+shellQuote(folder)+" ~/.local/bin").Run(); err != nil {
		return err
	}
	if err := command("scp", files[0], host+":"+folder+"/").Run(); err != nil {
		return err
	}
	if err := command("scp", bin, host+":/tmp/safeenv").Run(); err != nil {
		return err
	}
	return command("ssh", host, "install -m 755 /tmp/safeenv /usr/local/bin/safeenv 2>/dev/null || sudo install -m 755 /tmp/safeenv /usr/local/bin/safeenv || install -m 755 /tmp/safeenv ~/.local/bin/safeenv").Run()
}

func pull(host, folder, key string) error {
	out, err := exec.Command("ssh", host, "cd "+shellQuote(folder)+" && ls -1 *.safeenv").CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w: %s", err, out)
	}
	files := strings.Fields(string(out))
	if len(files) == 0 {
		return errors.New("no *.safeenv found")
	}
	for _, f := range files {
		if err := command("scp", host+":"+filepath.Join(folder, f), ".").Run(); err != nil {
			return err
		}
	}
	if key == "" {
		return nil
	}
	if len(files) != 1 {
		return errors.New("--decrypt needs exactly one *.safeenv")
	}
	env, err := decryptBytes(files[0], key)
	if err != nil {
		return err
	}
	return os.WriteFile(".env.decrypted", env, 0600)
}

func decrypt(path string) error {
	if err := ensureAge(); err != nil {
		return err
	}
	secret := privateKey()
	if secret == "" {
		return errors.New("SAFEENV_PRIVATE_KEY is required")
	}
	env, err := decryptBytes(path, secret)
	if err != nil {
		return err
	}
	envFile := fmt.Sprintf(".env.safeenv.%d", time.Now().UnixNano())
	if err := os.WriteFile(envFile, env, 0600); err != nil {
		return err
	}
	fmt.Println(envFile)
	return nil
}

func runWithEnv(args []string) error {
	if len(args) < 3 || args[1] != "--" {
		return usage()
	}
	secret := privateKey()
	if secret == "" {
		return errors.New("SAFEENV_PRIVATE_KEY is required")
	}
	env, err := decryptBytes(args[0], secret)
	if err != nil {
		return err
	}
	extra, err := parseEnv(env)
	if err != nil {
		return err
	}
	c := command(args[2], args[3:]...)
	c.Env = append(os.Environ(), extra...)
	return c.Run()
}

func decryptBytes(path, secret string) ([]byte, error) {
	if err := ensureAge(); err != nil {
		return nil, err
	}
	id, cleanup, err := identityFile(secret)
	if err != nil {
		return nil, err
	}
	defer cleanup()

	src, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	c := exec.Command("age", "-d", "-i", id)
	c.Stdin = bytes.NewReader(safeenvPayload(src))
	env, err := c.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("%w: %s", err, env)
	}
	return env, nil
}

func privateKey() string {
	return os.Getenv("SAFEENV_PRIVATE_KEY")
}

func parseEnv(src []byte) ([]string, error) {
	var out []string
	for i, line := range strings.Split(string(src), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		k, v, ok := strings.Cut(line, "=")
		if !ok || strings.TrimSpace(k) == "" {
			return nil, fmt.Errorf("bad env line %d", i+1)
		}
		k = strings.TrimSpace(k)
		v = strings.TrimSpace(v)
		if len(v) >= 2 {
			q := v[0]
			if (q == '"' || q == '\'') && v[len(v)-1] == q {
				v = v[1 : len(v)-1]
			}
		}
		out = append(out, k+"="+v)
	}
	return out, nil
}

func clean() error {
	files, err := filepath.Glob(".env.safeenv.*")
	if err != nil {
		return err
	}
	for _, f := range files {
		if err := os.Remove(f); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	return nil
}

func identityFile(secret string) (string, func(), error) {
	if strings.Contains(secret, "AGE-SECRET-KEY-") {
		f, err := os.CreateTemp("", "safeenv-*.key")
		if err != nil {
			return "", func() {}, err
		}
		if _, err := f.WriteString(secret + "\n"); err != nil {
			f.Close()
			os.Remove(f.Name())
			return "", func() {}, err
		}
		if err := f.Close(); err != nil {
			os.Remove(f.Name())
			return "", func() {}, err
		}
		return f.Name(), func() { _ = os.Remove(f.Name()) }, nil
	}
	return secret, func() {}, nil
}

func safeenvPayload(src []byte) []byte {
	return bytes.TrimPrefix(src, []byte(safeenvHeader))
}

func safeenvPath(path string) string {
	return path + ".safeenv"
}

func pullArgs(args []string) (string, string, string, error) {
	if len(args) < 2 || len(args) > 3 {
		return "", "", "", usage()
	}
	key := ""
	if len(args) == 3 {
		if !strings.HasPrefix(args[2], "--decrypt=") {
			return "", "", "", usage()
		}
		key = strings.TrimPrefix(args[2], "--decrypt=")
		if key == "" {
			return "", "", "", usage()
		}
	}
	return args[0], args[1], key, nil
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}

func safeenvBinary() (string, error) {
	if runtime.GOOS == "linux" {
		return os.Executable()
	}
	out := filepath.Join(os.TempDir(), "safeenv")
	c := exec.Command("go", "build", "-o", out, ".")
	c.Env = append(os.Environ(), "GOOS=linux", "GOARCH=amd64")
	c.Stdout, c.Stderr = os.Stdout, os.Stderr
	return out, c.Run()
}

func command(name string, args ...string) *exec.Cmd {
	c := exec.Command(name, args...)
	c.Stdin, c.Stdout, c.Stderr = os.Stdin, os.Stdout, os.Stderr
	return c
}
