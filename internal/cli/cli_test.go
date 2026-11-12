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

func TestPinCommands(t *testing.T) {
	store := history.Store{Path: filepath.Join(t.TempDir(), "history.json")}
	var out, stderr bytes.Buffer
	app := App{Store: store, Out: &out, Err: &stderr}
	run := func(args ...string) int { out.Reset(); stderr.Reset(); return app.Run(args) }
	dir := t.TempDir()
	if run("pin", "Favorite", dir) != 0 {
		t.Fatal(stderr.String())
	}
	if run("query", "FAVORITE") != 0 {
		t.Fatal(stderr.String())
	}
	want, _ := history.Canonical(dir)
	if strings.TrimSpace(out.String()) != want {
		t.Fatal(out.String())
	}
	if run("pins") != 0 || !strings.HasPrefix(out.String(), "favorite\t") {
		t.Fatal(out.String(), stderr.String())
	}
	if run("query", "--pin", "--", "absent") != 2 || out.Len() != 0 || stderr.Len() != 0 {
		t.Fatal("missing pin must return silent exit 2")
	}
	if run("pin", "favorite", t.TempDir()) == 0 {
		t.Fatal("replaced without flag")
	}
	if run("pin", "--replace", "favorite", dir) != 0 {
		t.Fatal(stderr.String())
	}
	if run("unpin", "FAVORITE") != 0 {
		t.Fatal(stderr.String())
	}
	for _, args := range [][]string{{"pin"}, {"pins", "extra"}, {"unpin"}, {"query", "--pin"}} {
		if run(args...) == 0 {
			t.Fatal("accepted invalid args", args)
		}
	}
}

func TestZshPinBeatsLocalDirectoryAndMissingPinDoesNotFallback(t *testing.T) {
	zsh, err := exec.LookPath("zsh")
	if err != nil {
		if os.Getenv("CI") == "true" {
			t.Fatal(err)
		}
		t.Skip("requires Zsh")
	}
	binary := buildBinary(t)
	home, data := t.TempDir(), t.TempDir()
	target := filepath.Join(t.TempDir(), "different name 'quote $(touch PWNED)")
	if err := os.Mkdir(target, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(home, "favorite"), 0o700); err != nil {
		t.Fatal(err)
	}
	want, _ := history.Canonical(target)
	script := `eval "$(pogo init zsh)"
pogo pin favorite "$TARGET" || exit 21
j favorite || exit 22
[[ "$PWD" == "$EXPECTED" ]] || exit 23
cd -- "$HOME" || exit 24
j ./favorite || exit 25
[[ "$PWD" == "$HOME/favorite" ]] || exit 26
cd -- "$HOME" || exit 27
rmdir -- "$TARGET" || exit 28
if j favorite >/dev/null 2>&1; then exit 29; fi
[[ "$PWD" == "$HOME" && ! -e PWNED ]] || exit 30
`
	cmd := exec.Command(zsh, "-d", "-f", "-c", script)
	cmd.Dir = home
	cmd.Env = append(os.Environ(), "HOME="+home, "XDG_DATA_HOME="+data,
		"PATH="+filepath.Dir(binary)+":/usr/bin:/bin", "TARGET="+target, "EXPECTED="+want)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("pin shell integration: %v\n%s", err, output)
	}
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
		if os.Getenv("CI") == "true" {
			t.Fatal("CI must provide Zsh for the integration test")
		}
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
