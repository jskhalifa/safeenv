package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

const safeenvHeader = "SAFEENV AGE V1\n"
const version = "0.1.1"
const releaseRepo = "jskhalifa/safeenv"

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
	keyName := "SAFEENV_PRIVATE_KEY"
	if strings.HasPrefix(args[0], "--env-key-name=") {
		keyName = strings.TrimPrefix(args[0], "--env-key-name=")
		if keyName == "" {
			return usage()
		}
		args = args[1:]
		if len(args) == 0 {
			return usage()
		}
	}
	if args[0] == "--help" || args[0] == "-h" {
		fmt.Print(help())
		return nil
	}
	if args[0] == "--version" || args[0] == "version" {
		fmt.Println(version)
		return nil
	}
	switch args[0] {
	case "init":
		if len(args) < 2 || len(args) > 3 {
			return usage()
		}
		doKeygen := false
		if len(args) == 3 {
			if args[2] != "--keygen" {
				return usage()
			}
			doKeygen = true
		}
		return initEnv(args[1], doKeygen)
	case "keygen":
		name := "secrets"
		if len(args) > 2 {
			return usage()
		}
		if len(args) == 2 {
			name = args[1]
		}
		return keyGen(name)
	case "encrypt":
		if len(args) < 2 {
			return usage()
		}
		return encrypt(args[1], args[2:])
	case "push":
		if len(args) != 4 {
			return usage()
		}
		return push(args[1], args[2], args[3])
	case "list":
		if len(args) != 1 {
			return usage()
		}
		return listFiles()
	case "inspect":
		if len(args) != 2 {
			return usage()
		}
		return inspect(args[1])
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
		key, path, err := decryptArgs(args[1:])
		if err != nil {
			return err
		}
		if path == "" {
			path = os.Getenv("ENV_SECRET_FILE")
		}
		if path == "" {
			return usage()
		}
		return decrypt(path, key, keyName)
	default:
		return runWithEnv(args, keyName)
	}
}

func usage() error {
	return errors.New("usage: safeenv [--env-key-name=ENV] [--version|--help] | safeenv [--env-key-name=ENV] [-i key|-] <file.safeenv> -- <command> [args...] | init <file.env> [--keygen] | keygen [name] | encrypt <file.env> [--public-key agepub...] | push <file.safeenv> <user>@<ip> <folder> | pull <user>@<ip> <folder> [--decrypt=<secret.key>] | decrypt [-i key|-] [file.safeenv] | list | inspect <file.safeenv> | clean")
}

func help() string {
	return `safeenv ` + version + `

Usage:
  safeenv init <file.env> [--keygen]
  safeenv keygen [name]
  safeenv encrypt <file.env> [--public-key age1...]
  safeenv push <file.safeenv> <user>@<ip> <folder>
  safeenv pull <user>@<ip> <folder> [--decrypt=<secret.key>]
  safeenv [--env-key-name=ENV] [-i <identity-file>|-i -] <file.safeenv> -- <command> [args...]
  safeenv decrypt [-i <identity-file>|-i -] [file.safeenv]
  safeenv list
  safeenv inspect <file.safeenv>
  safeenv clean
  safeenv --version

Private key sources:
  SAFEENV_PRIVATE_KEY      raw age identity or path to identity file
  --env-key-name=ENV       read private key from ENV instead
  -i <file>               age identity file
  -i -                    read age identity from stdin
`
}

func initEnv(path string, doKeygen bool) error {
	logEvent("init", "start", path, nil, "")
	if err := writeFile(path, nil, 0600); err != nil {
		logEvent("init", "fail", path, nil, err.Error())
		return err
	}
	err := appendGitignore([]string{
		path,
		safeenvPath(path),
		path + ".key",
		path + ".pub",
		".env.safeenv.*",
		".env.decrypted",
		"safeenv",
	})
	if err != nil {
		logEvent("init", "fail", path, nil, err.Error())
		return err
	}
	if doKeygen {
		if err := keyGen(path); err != nil {
			logEvent("init", "fail", path, nil, err.Error())
			return err
		}
	}
	logEvent("init", "ok", path, nil, "")
	return nil
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

func writeFile(path string, data []byte, perm os.FileMode) error {
	if err := backup(path); err != nil {
		return err
	}
	return os.WriteFile(path, data, perm)
}

func backup(path string) error {
	if _, err := os.Stat(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	dst := fmt.Sprintf("%s.%d.bak", path, time.Now().Unix())
	logEvent("backup", "ok", path, nil, "backup="+dst)
	return os.Rename(path, dst)
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
	logEvent("keygen", "start", name, nil, "")
	if err := ensureAge(); err != nil {
		logEvent("keygen", "fail", name, nil, err.Error())
		return err
	}
	key, pub := name+".key", name+".pub"
	if err := backup(key); err != nil {
		return err
	}
	if err := backup(pub); err != nil {
		return err
	}
	if err := command("age-keygen", "-o", key).Run(); err != nil {
		logEvent("keygen", "fail", name, nil, err.Error())
		return err
	}
	out, err := exec.Command("age-keygen", "-y", key).Output()
	if err != nil {
		logEvent("keygen", "fail", name, nil, err.Error())
		return err
	}
	if err := writeFile(pub, out, 0644); err != nil {
		logEvent("keygen", "fail", name, nil, err.Error())
		return err
	}
	err = os.Chmod(key, 0600)
	if err != nil {
		logEvent("keygen", "fail", name, nil, err.Error())
		return err
	}
	logEvent("keygen", "ok", name, nil, "")
	return nil
}

func encrypt(path string, args []string) error {
	logEvent("encrypt", "start", path, nil, "")
	var ageRecipients []string
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--public-key", "-r", "--recipient":
			i++
			if i >= len(args) {
				return usage()
			}
			ageRecipients = append(ageRecipients, args[i])
		default:
			return usage()
		}
	}

	if err := ensureAge(); err != nil {
		logEvent("encrypt", "fail", path, nil, err.Error())
		return err
	}
	ageArgs := []string{}
	if fileExists(path + ".pub") {
		ageArgs = append(ageArgs, "-R", path+".pub")
	} else if len(ageRecipients) == 0 {
		err := fmt.Errorf("%s not found; pass public key with --public-key age1...", path+".pub")
		logEvent("encrypt", "fail", path, nil, err.Error())
		return err
	}
	for _, r := range ageRecipients {
		ageArgs = append(ageArgs, "-r", r)
	}
	ageArgs = append(ageArgs, path)
	out, err := exec.Command("age", ageArgs...).CombinedOutput()
	if err != nil {
		logEvent("encrypt", "fail", path, nil, string(out))
		return fmt.Errorf("%w: %s", err, out)
	}
	err = writeFile(safeenvPath(path), append([]byte(safeenvHeader), out...), 0600)
	if err != nil {
		logEvent("encrypt", "fail", path, nil, err.Error())
		return err
	}
	logEvent("encrypt", "ok", safeenvPath(path), nil, fmt.Sprintf("age_recipients=%d", 1+len(ageRecipients)))
	return nil
}

func push(file, host, folder string) error {
	logEvent("push", "start", file, []string{"ssh", host}, "")
	if !strings.HasSuffix(file, ".safeenv") {
		err := errors.New("push only accepts encrypted *.safeenv files")
		logEvent("push", "fail", file, nil, err.Error())
		return err
	}
	if _, err := os.Stat(file); err != nil {
		logEvent("push", "fail", file, nil, err.Error())
		return err
	}
	goos, goarch, err := target(host)
	if err != nil {
		logEvent("push", "fail", file, nil, err.Error())
		return err
	}
	asset := releaseURL(goos, goarch)
	if err := command("ssh", host, "mkdir -p "+shellQuote(folder)).Run(); err != nil {
		logEvent("push", "fail", file, nil, err.Error())
		return err
	}
	if err := command("scp", file, host+":"+folder+"/").Run(); err != nil {
		logEvent("push", "fail", file, nil, err.Error())
		return err
	}
	install := "tmp=$(mktemp) && " +
		"(curl -fsSL -o \"$tmp\" " + shellQuote(asset) + " || wget -qO \"$tmp\" " + shellQuote(asset) + ") && " +
		"(install -m 755 \"$tmp\" /usr/local/bin/safeenv 2>/dev/null || sudo install -m 755 \"$tmp\" /usr/local/bin/safeenv); " +
		"status=$?; rm -f \"$tmp\"; exit $status"
	err = command("ssh", host, install).Run()
	if err != nil {
		logEvent("push", "fail", file, nil, err.Error())
		return err
	}
	logEvent("push", "ok", file, nil, "installed=/usr/local/bin/safeenv asset="+asset)
	return nil
}

func pull(host, folder, key string) error {
	logEvent("pull", "start", folder, []string{"ssh", host}, "")
	out, err := exec.Command("ssh", host, "cd "+shellQuote(folder)+" && ls -1 *.safeenv").CombinedOutput()
	if err != nil {
		logEvent("pull", "fail", folder, nil, string(out))
		return fmt.Errorf("%w: %s", err, out)
	}
	files := strings.Fields(string(out))
	if len(files) == 0 {
		return errors.New("no *.safeenv found")
	}
	for _, f := range files {
		if err := command("scp", host+":"+filepath.Join(folder, f), ".").Run(); err != nil {
			logEvent("pull", "fail", f, nil, err.Error())
			return err
		}
	}
	if key == "" {
		logEvent("pull", "ok", folder, nil, fmt.Sprintf("files=%d", len(files)))
		return nil
	}
	if len(files) != 1 {
		return errors.New("--decrypt needs exactly one *.safeenv")
	}
	env, err := decryptBytes(files[0], key)
	if err != nil {
		logEvent("pull", "fail", files[0], nil, err.Error())
		return err
	}
	err = writeFile(".env.decrypted", env, 0600)
	if err != nil {
		logEvent("pull", "fail", ".env.decrypted", nil, err.Error())
		return err
	}
	logEvent("pull", "ok", ".env.decrypted", nil, "")
	return nil
}

func decrypt(path, key, keyName string) error {
	logEvent("decrypt", "start", path, nil, "mode=tempfile version="+fileVersion(path))
	secret, err := privateKey(key, keyName)
	if err != nil {
		logEvent("decrypt", "fail", path, nil, err.Error())
		return err
	}
	if secret == "" {
		msg := keyName + " is required"
		logEvent("decrypt", "fail", path, nil, msg)
		return errors.New(msg)
	}
	env, err := decryptBytes(path, secret)
	if err != nil {
		logEvent("decrypt", "fail", path, nil, err.Error())
		return err
	}
	envFile := fmt.Sprintf(".env.safeenv.%d", time.Now().UnixNano())
	if err := writeFile(envFile, env, 0600); err != nil {
		logEvent("decrypt", "fail", envFile, nil, err.Error())
		return err
	}
	fmt.Println(envFile)
	logEvent("decrypt", "ok", path, nil, "output="+envFile)
	return nil
}

func runWithEnv(args []string, keyName string) error {
	key := ""
	if len(args) >= 2 && args[0] == "-i" {
		key = args[1]
		args = args[2:]
	}
	if len(args) < 3 || args[1] != "--" {
		return usage()
	}
	secret, err := privateKey(key, keyName)
	if err != nil {
		logEvent("run", "fail", args[0], args[2:], err.Error())
		return err
	}
	if secret == "" {
		msg := keyName + " is required"
		logEvent("run", "fail", args[0], args[2:], msg)
		return errors.New(msg)
	}
	env, err := decryptBytes(args[0], secret)
	if err != nil {
		logEvent("run", "fail", args[0], args[2:], err.Error())
		return err
	}
	extra, err := parseEnv(env)
	if err != nil {
		logEvent("run", "fail", args[0], args[2:], err.Error())
		return err
	}
	if isDockerCompose(args[2:]) {
		logEvent("run", "warn", args[0], args[2:], "docker compose up has no -e flag; env is passed to compose process for interpolation")
	}
	logEvent("run", "start", args[0], args[2:], fmt.Sprintf("vars=%d version=%s", len(extra), fileVersion(args[0])))
	c := command(args[2], args[3:]...)
	c.Env = append(os.Environ(), extra...)
	err = c.Run()
	if err != nil {
		logEvent("run", "fail", args[0], args[2:], err.Error())
		return err
	}
	logEvent("run", "ok", args[0], args[2:], "")
	return nil
}

func decryptBytes(path, secret string) ([]byte, error) {
	src, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	if err := ensureAge(); err != nil {
		return nil, err
	}
	id, cleanup, err := identityFile(secret)
	if err != nil {
		return nil, err
	}
	defer cleanup()

	c := exec.Command("age", "-d", "-i", id)
	c.Stdin = bytes.NewReader(safeenvPayload(src))
	env, err := c.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("%w: %s", err, env)
	}
	return env, nil
}

func privateKey(flag, keyName string) (string, error) {
	if flag == "-" {
		b, err := io.ReadAll(os.Stdin)
		if err != nil {
			return "", err
		}
		warnWeakKey(string(b))
		return string(b), nil
	}
	if flag != "" {
		return flag, nil
	}
	v := os.Getenv(keyName)
	warnWeakKey(v)
	return v, nil
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
	logEvent("clean", "start", ".env.safeenv.*", nil, "")
	files, err := filepath.Glob(".env.safeenv.*")
	if err != nil {
		logEvent("clean", "fail", ".env.safeenv.*", nil, err.Error())
		return err
	}
	for _, f := range files {
		if err := os.Remove(f); err != nil && !errors.Is(err, os.ErrNotExist) {
			logEvent("clean", "fail", f, nil, err.Error())
			return err
		}
	}
	logEvent("clean", "ok", ".env.safeenv.*", nil, fmt.Sprintf("files=%d", len(files)))
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

func fileVersion(path string) string {
	src, err := os.ReadFile(path)
	if err != nil {
		return "unknown"
	}
	if bytes.HasPrefix(src, []byte(safeenvHeader)) {
		return strings.TrimSpace(strings.TrimPrefix(safeenvHeader, "SAFEENV "))
	}
	return "age"
}

func safeenvPath(path string) string {
	return path + ".safeenv"
}

func decryptArgs(args []string) (string, string, error) {
	key := ""
	if len(args) >= 2 && args[0] == "-i" {
		key = args[1]
		args = args[2:]
	}
	if len(args) > 1 {
		return "", "", usage()
	}
	path := ""
	if len(args) == 1 {
		path = args[0]
	}
	return key, path, nil
}

func listFiles() error {
	files := map[string]bool{}
	add := func(pattern string) error {
		matches, err := filepath.Glob(pattern)
		if err != nil {
			return err
		}
		for _, f := range matches {
			if strings.HasSuffix(f, ".key") || strings.HasSuffix(f, ".pub") {
				continue
			}
			files[f] = true
		}
		return nil
	}
	if err := add("*.safeenv"); err != nil {
		return err
	}
	if err := add(".env.*"); err != nil {
		return err
	}
	var sorted []string
	for f := range files {
		sorted = append(sorted, f)
	}
	sort.Strings(sorted)
	width := len("file")
	for _, f := range sorted {
		if len(f) > width {
			width = len(f)
		}
	}
	fmt.Printf("%-*s    %-8s    %8s    %-9s    %s\n", width, "FILE", "FORMAT", "SIZE", "ENCRYPTED", "KEYS")
	for _, f := range sorted {
		info, err := os.Stat(f)
		if err != nil {
			continue
		}
		encrypted := "no"
		format := "-"
		if strings.HasSuffix(f, ".safeenv") {
			encrypted = "yes"
			format = fileVersion(f)
		}
		fmt.Printf("%-*s    %-8s    %8d    %-9s    %s\n", width, f, format, info.Size(), encrypted, keyStatus(f))
	}
	return nil
}

func keyStatus(path string) string {
	base := strings.TrimSuffix(path, ".safeenv")
	hasPub := fileExists(base + ".pub")
	hasKey := fileExists(base + ".key")
	switch {
	case hasPub && hasKey:
		return "public-private"
	case hasPub:
		return "public"
	case hasKey:
		return "private"
	default:
		return "no"
	}
}

func inspect(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		logEvent("inspect", "fail", path, nil, err.Error())
		return err
	}
	fmt.Printf("file\t%s\n", path)
	fmt.Printf("format\t%s\n", fileVersion(path))
	fmt.Printf("size\t%d\n", info.Size())
	logEvent("inspect", "ok", path, nil, "")
	return nil
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

func logEvent(action, status, file string, cmd []string, msg string) {
	parts := []string{
		"time=" + strconv.Quote(time.Now().Format(time.RFC3339)),
		"action=" + strconv.Quote(action),
		"status=" + strconv.Quote(status),
	}
	if file != "" {
		parts = append(parts, "file="+strconv.Quote(file))
	}
	if len(cmd) > 0 {
		parts = append(parts, "command="+strconv.Quote(strings.Join(cmd, " ")))
	}
	if msg != "" {
		parts = append(parts, "msg="+strconv.Quote(msg))
	}
	fmt.Fprintln(os.Stderr, strings.Join(parts, " "))
}

func warnWeakKey(secret string) {
	if secret == "" {
		return
	}
	if strings.Contains(secret, "AGE-SECRET-KEY-") || fileExists(secret) {
		return
	}
	logEvent("key", "warn", "", nil, "private key is not an age identity or readable identity file")
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func isDockerCompose(cmd []string) bool {
	if len(cmd) >= 2 && cmd[0] == "docker" && cmd[1] == "compose" {
		return true
	}
	return len(cmd) >= 1 && cmd[0] == "docker-compose"
}

func target(host string) (string, string, error) {
	out, err := exec.Command("ssh", host, "uname -s; uname -m").CombinedOutput()
	if err != nil {
		return "", "", fmt.Errorf("%w: %s", err, out)
	}
	lines := strings.Fields(strings.ToLower(string(out)))
	if len(lines) < 2 {
		return "", "", fmt.Errorf("cannot detect target from %q", out)
	}
	goos, goarch := mapTarget(lines[0], lines[1])
	if goos == "" {
		return "", "", fmt.Errorf("unsupported target OS %q", lines[0])
	}
	if goarch == "" {
		return "", "", fmt.Errorf("unsupported target arch %q", lines[1])
	}
	return goos, goarch, nil
}

func mapTarget(osName, archName string) (string, string) {
	return map[string]string{"linux": "linux"}[osName], map[string]string{
		"x86_64":  "amd64",
		"amd64":   "amd64",
		"aarch64": "arm64",
		"arm64":   "arm64",
	}[archName]
}

func releaseURL(goos, goarch string) string {
	return fmt.Sprintf("https://github.com/%s/releases/download/%s/safeenv-%s-%s", releaseRepo, version, goos, goarch)
}

func command(name string, args ...string) *exec.Cmd {
	c := exec.Command(name, args...)
	c.Stdin, c.Stdout, c.Stderr = os.Stdin, os.Stdout, os.Stderr
	return c
}
