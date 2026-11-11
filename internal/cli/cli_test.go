package cli

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/filipbodor/pogo/internal/history"
)

func TestCLIHistoryLifecycle(t *testing.T) {
	store := history.Store{Path: filepath.Join(t.TempDir(), "data", "history.json")}
	var out, stderr bytes.Buffer
	app := App{Store: store, Out: &out, Err: &stderr, Version: "test"}
	run := func(args ...string) (int, string) {
		t.Helper()
		out.Reset()
		stderr.Reset()
		code := app.Run(args)
		return code, strings.TrimSpace(out.String())
	}
	if code, text := run("query", "unseen"); code == 0 || text != "" {
		t.Fatal("an unknown directory should not produce a destination")
	}
	dir := filepath.Join(t.TempDir(), "Project with spaces")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if code, _ := run("add", "--", dir); code != 0 {
		t.Fatal(stderr.String())
	}
	want, _ := history.Canonical(dir)
	for _, command := range []string{"query", "list"} {
		if code, text := run(command, "PROJECT", "spaces"); code != 0 || text != want {
			t.Fatalf("%s: code=%d output=%q err=%s", command, code, text, stderr.String())
		}
	}
	if code, text := run("--version"); code != 0 || text != "pogo test" {
		t.Fatalf("unexpected version: %d, %s", code, text)
	}
	if code, _ := run("remove", dir); code != 0 {
		t.Fatal(stderr.String())
	}
	if code, text := run("list"); code != 0 || text != "" {
		t.Fatal("removed directory remains in history")
	}
	for _, args := range [][]string{{"add", "a", "b"}, {"init", "fish"}, {"remove"}, {"prune", "a"}, {"bogus"}} {
		if code, _ := run(args...); code == 0 {
			t.Fatalf("invalid command accepted: %v", args)
		}
	}
}

func buildBinary(t *testing.T) string {
	t.Helper()
	binary := filepath.Join(t.TempDir(), "pogo")
	cmd := exec.Command("go", "build", "-o", binary, "../../cmd/pogo")
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, output)
	}
	return binary
}

func TestSeparateProcessesDoNotLoseVisits(t *testing.T) {
	binary := buildBinary(t)
	data := t.TempDir()
	dir := t.TempDir()
	var commands []*exec.Cmd
	for range 20 {
		cmd := exec.Command(binary, "add", "--", dir)
		cmd.Env = append(os.Environ(), "XDG_DATA_HOME="+data)
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		commands = append(commands, cmd)
	}
	for _, cmd := range commands {
		if err := cmd.Wait(); err != nil {
			t.Error(err)
		}
	}
	db, err := (history.Store{Path: filepath.Join(data, "pogo", "history.json")}).Read()
	if err != nil || len(db.Entries) != 1 || db.Entries[0].Visits != 20 {
		t.Fatalf("lost process updates: %+v, %v", db, err)
	}
}

func TestZshJumpPreservesHooksAndTreatsPathsAsData(t *testing.T) {
	zsh, err := exec.LookPath("zsh")
	if err != nil {
		t.Skip("zsh is required for integration test")
	}
	binary := buildBinary(t)
	home := t.TempDir()
	data := t.TempDir()
	target := filepath.Join(t.TempDir(), "app 'quote $(touch PWNED); literal")
	if err := os.Mkdir(target, 0o700); err != nil {
		t.Fatal(err)
	}
	expected, err := history.Canonical(target)
	if err != nil {
		t.Fatal(err)
	}
	script := `function _existing() { print -r -- x >> "$HOME/hooks"; }
chpwd_functions=(_existing)
eval "$(pogo init zsh)"
eval "$(pogo init zsh)"
cd -- "$TARGET" || exit 11
cd -- "$HOME" || exit 12
j literal || exit 13
[[ "$PWD" == "$EXPECTED" ]] || exit 14
before="$PWD"
if j definitely-unseen > /dev/null 2>&1; then exit 15; fi
[[ "$PWD" == "$before" ]] || exit 16
[[ ! -e PWNED ]] || exit 17
`
	cmd := exec.Command(zsh, "-d", "-f", "-c", script)
	cmd.Dir = home
	cmd.Env = append(os.Environ(), "HOME="+home, "XDG_DATA_HOME="+data,
		"PATH="+filepath.Dir(binary)+":/usr/bin:/bin", "TARGET="+target, "EXPECTED="+expected)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("shell integration: %v\n%s", err, output)
	}
	hooks, err := os.ReadFile(filepath.Join(home, "hooks"))
	if err != nil || string(hooks) != "x\nx\nx\n" {
		t.Fatalf("existing hook changed: %q, %v", hooks, err)
	}
	db, err := (history.Store{Path: filepath.Join(data, "pogo", "history.json")}).Read()
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, entry := range db.Entries {
		if entry.Path == expected {
			found = true
			if entry.Visits != 2 {
				t.Fatalf("reinitializing duplicated hooks: visits=%d", entry.Visits)
			}
		}
	}
	if !found {
		t.Fatal("directory changes were not learned")
	}
}
