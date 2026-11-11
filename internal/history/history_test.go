package history

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func testStore(t *testing.T) Store {
	t.Helper()
	return Store{Path: filepath.Join(t.TempDir(), "data", "history.json")}
}

func TestDefaultPathAndRelativeXDGRejection(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_DATA_HOME", "")
	path, err := DefaultPath()
	if err != nil || path != filepath.Join(home, ".local", "share", "pogo", "history.json") {
		t.Fatalf("unexpected default: %s, %v", path, err)
	}
	t.Setenv("XDG_DATA_HOME", "relative")
	if _, err := DefaultPath(); err == nil {
		t.Fatal("relative XDG data path accepted")
	}
}

func TestLockContentionFailsWithoutChangingHistory(t *testing.T) {
	s := testStore(t)
	dir := t.TempDir()
	if err := s.Add(dir, time.Now()); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(s.Path)
	unlock, err := lock(s.Path + ".lock")
	if err != nil {
		t.Fatal(err)
	}
	defer unlock()
	started := time.Now()
	if err := s.Add(dir, time.Now()); err == nil {
		t.Fatal("expected lock contention error")
	}
	if time.Since(started) > 3*time.Second {
		t.Fatal("lock contention blocked too long")
	}
	after, _ := os.ReadFile(s.Path)
	if string(before) != string(after) {
		t.Fatal("history changed without the lock")
	}
}

func TestConcurrentVisitsAndPrivatePermissions(t *testing.T) {
	s := testStore(t)
	dir := t.TempDir()
	var workers sync.WaitGroup
	for range 30 {
		workers.Go(func() {
			if err := s.Add(dir, time.Now()); err != nil {
				t.Error(err)
			}
		})
	}
	workers.Wait()
	db, err := s.Read()
	if err != nil || len(db.Entries) != 1 || db.Entries[0].Visits != 30 {
		t.Fatalf("lost concurrent updates: %+v, %v", db, err)
	}
	for path, mode := range map[string]os.FileMode{s.Path: 0o600, filepath.Dir(s.Path): 0o700} {
		info, err := os.Stat(path)
		if err != nil || info.Mode().Perm() != mode {
			t.Fatalf("unexpected private permissions for %s: %v, %v", path, info, err)
		}
	}
}

func TestCorruptAndFutureDataArePreserved(t *testing.T) {
	for _, content := range []string{"broken JSON", `{"version":99}`, `{"version":1,"entries":[{"path":"relative"}]}`} {
		t.Run(content, func(t *testing.T) {
			s := testStore(t)
			if err := os.MkdirAll(filepath.Dir(s.Path), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(s.Path, []byte(content), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := s.Add(t.TempDir(), time.Now()); err == nil {
				t.Fatal("expected invalid history to fail")
			}
			data, _ := os.ReadFile(s.Path)
			if string(data) != content {
				t.Fatal("original history changed")
			}
		})
	}
}

func TestSymlinkAndUnsupportedNames(t *testing.T) {
	dir := t.TempDir()
	link := filepath.Join(t.TempDir(), "alias")
	if err := os.Symlink(dir, link); err != nil {
		t.Fatal(err)
	}
	s := testStore(t)
	for _, path := range []string{dir, link} {
		if err := s.Add(path, time.Now()); err != nil {
			t.Fatal(err)
		}
	}
	db, _ := s.Read()
	if len(db.Entries) != 1 || db.Entries[0].Visits != 2 {
		t.Fatalf("aliases not merged: %+v", db)
	}
	bad := filepath.Join(t.TempDir(), "bad\nname")
	if err := os.Mkdir(bad, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := s.Add(bad, time.Now()); err == nil {
		t.Fatal("newline directory should fail")
	}
	badLink := filepath.Join(t.TempDir(), "innocent")
	if err := os.Symlink(bad, badLink); err != nil {
		t.Fatal(err)
	}
	if err := s.Add(badLink, time.Now()); err == nil {
		t.Fatal("newline symlink target should fail")
	}
}

func TestRankingMatchingAndStaleDirectories(t *testing.T) {
	root := t.TempDir()
	var entries []Entry
	now := time.Now()
	for name, age := range map[string]time.Duration{"Project": 0, "project-old": 90 * 24 * time.Hour} {
		path := filepath.Join(root, name)
		if err := os.Mkdir(path, 0o700); err != nil {
			t.Fatal(err)
		}
		entries = append(entries, Entry{Path: path, Visits: 3, LastVisited: now.Add(-age)})
	}
	entries = append(entries, Entry{Path: filepath.Join(root, "missing"), Visits: 1000, LastVisited: now})
	matches := Rank(entries, []string{"PROJ"}, now)
	if len(matches) != 2 || filepath.Base(matches[0].Path) != "Project" {
		t.Fatalf("recency/case matching incorrect: %+v", matches)
	}
	if len(Rank(entries, []string{"Project", filepath.Base(root)}, now)) != 0 {
		t.Fatal("terms must occur in order")
	}
	if len(Rank(entries, []string{".*"}, now)) != 0 {
		t.Fatal("terms must be literal substrings")
	}
}

func TestRemoveAndPruneStalePaths(t *testing.T) {
	s := testStore(t)
	dir := filepath.Join(t.TempDir(), "gone")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := s.Add(dir, time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(dir); err != nil {
		t.Fatal(err)
	}
	if err := s.Remove(dir); err != nil {
		t.Fatal(err)
	}
	db, _ := s.Read()
	if len(db.Entries) != 0 {
		t.Fatal("stale entry was not removed")
	}
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := s.Add(dir, time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(dir); err != nil {
		t.Fatal(err)
	}
	if err := s.Prune(); err != nil {
		t.Fatal(err)
	}
	db, _ = s.Read()
	if len(db.Entries) != 0 {
		t.Fatal("stale entry was not pruned")
	}
}
