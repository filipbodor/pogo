package history

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestPinsOverrideRankingAndPreserveHistory(t *testing.T) {
	s := testStore(t)
	favorite := t.TempDir()
	learned := filepath.Join(t.TempDir(), "work")
	if err := os.Mkdir(learned, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := s.Add(learned, time.Now()); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(s.Path)
	if err := s.Pin("Work", favorite, false); err != nil {
		t.Fatal(err)
	}
	after, _ := os.ReadFile(s.Path)
	if string(before) != string(after) {
		t.Fatal("pinning changed learned history")
	}
	matches, err := s.Resolve([]string{"WORK"}, time.Now())
	want, _ := Canonical(favorite)
	if err != nil || len(matches) != 2 || matches[0].Path != want || matches[0].PinName != "work" {
		t.Fatalf("pin did not win: %+v, %v", matches, err)
	}
	info, _ := os.Stat(s.PinsPath())
	if info.Mode().Perm() != 0o600 {
		t.Fatal("pins are not private")
	}
	pinsBefore, _ := os.ReadFile(s.PinsPath())
	if err := s.Add(learned, time.Now()); err != nil {
		t.Fatal(err)
	}
	pinsAfter, _ := os.ReadFile(s.PinsPath())
	if string(pinsBefore) != string(pinsAfter) {
		t.Fatal("recording visits changed pins")
	}
	if err := s.Unpin("WORK"); err != nil {
		t.Fatal(err)
	}
	db, _ := s.Read()
	if len(db.Entries) != 1 || db.Entries[0].Visits != 2 {
		t.Fatal("unpinning changed visits")
	}
}

func TestPinReplacementAndValidation(t *testing.T) {
	s := testStore(t)
	first, second := t.TempDir(), t.TempDir()
	for _, name := range []string{"", "-", ".", "..", "two words", "with/slash", "bad\nname"} {
		if err := s.Pin(name, first, false); err == nil {
			t.Fatalf("accepted invalid name %q", name)
		}
	}
	if err := s.Pin("app", first, false); err != nil {
		t.Fatal(err)
	}
	if err := s.Pin("APP", first, false); err != nil {
		t.Fatal("same target should be idempotent", err)
	}
	if err := s.Pin("app", second, false); err == nil {
		t.Fatal("replaced pin without explicit flag")
	}
	if err := s.Pin("app", second, true); err != nil {
		t.Fatal(err)
	}
	pins, _ := s.Pins()
	want, _ := Canonical(second)
	if pins["app"] != want {
		t.Fatal("replacement not saved")
	}
	if err := s.Unpin("unknown"); err == nil {
		t.Fatal("unknown pin should report an error")
	}
}

func TestMissingPinNeverFallsBackAndPrunePreservesIt(t *testing.T) {
	s := testStore(t)
	target := filepath.Join(t.TempDir(), "gone")
	if err := os.Mkdir(target, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := s.Pin("app", target, false); err != nil {
		t.Fatal(err)
	}
	other := filepath.Join(t.TempDir(), "app")
	if err := os.Mkdir(other, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := s.Add(other, time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(target); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Resolve([]string{"app"}, time.Now()); err == nil {
		t.Fatal("missing pin fell back to a learned path")
	}
	if err := s.Prune(); err != nil {
		t.Fatal(err)
	}
	pins, _ := s.Pins()
	if len(pins) != 1 {
		t.Fatal("prune removed an explicit favorite")
	}
}

func TestCorruptPinsArePreserved(t *testing.T) {
	for _, data := range []string{"broken", `{"version":99}`, `{"version":1,"pins":{"app":"relative"}}`} {
		s := testStore(t)
		if err := os.MkdirAll(filepath.Dir(s.Path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(s.PinsPath(), []byte(data), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := s.Pin("app", t.TempDir(), true); err == nil {
			t.Fatal("accepted invalid pins")
		}
		got, _ := os.ReadFile(s.PinsPath())
		if string(got) != data {
			t.Fatal("invalid pins overwritten")
		}
	}
}

func TestConcurrentPinWrites(t *testing.T) {
	s := testStore(t)
	var workers sync.WaitGroup
	for _, name := range []string{"one", "two", "three", "four", "five"} {
		dir := t.TempDir()
		workers.Go(func() {
			if err := s.Pin(name, dir, false); err != nil {
				t.Error(err)
			}
		})
	}
	workers.Wait()
	pins, err := s.Pins()
	if err != nil || len(pins) != 5 {
		t.Fatalf("lost pins: %v, %v", pins, err)
	}
}
