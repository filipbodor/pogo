package cli

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/filipbodor/pogo/internal/history"
)

func TestExplainAgreesWithQueryAndDoesNotRecordVisits(t *testing.T) {
	store := history.Store{Path: filepath.Join(t.TempDir(), "history.json")}
	dir := filepath.Join(t.TempDir(), "client")
	if err := os.Mkdir(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := store.Add(dir, time.Now().Add(-7*24*time.Hour)); err != nil {
		t.Fatal(err)
	}
	var out, stderr bytes.Buffer
	app := App{Store: store, Out: &out, Err: &stderr}
	if app.Run([]string{"query", "client"}) != 0 {
		t.Fatal(stderr.String())
	}
	winner := strings.TrimSpace(out.String())
	before, _ := os.ReadFile(store.Path)
	out.Reset()
	if app.Run([]string{"explain", "client"}) != 0 {
		t.Fatal(stderr.String())
	}
	for _, text := range []string{fmt.Sprintf("Winner: %q", winner), "highest learned score", "visits: 1", "age: 7.00 days", "frequency 0.6931", "recency 0.5000", "basename 2", "score 0.6931"} {
		if !strings.Contains(out.String(), text) {
			t.Fatalf("missing %q in %s", text, out.String())
		}
	}
	after, _ := os.ReadFile(store.Path)
	if !bytes.Equal(before, after) {
		t.Fatal("explain modified visit history")
	}
	if err := store.Pin("favorite", dir, false); err != nil {
		t.Fatal(err)
	}
	pinsBefore, _ := os.ReadFile(store.PinsPath())
	out.Reset()
	if app.Run([]string{"explain", "FAVORITE"}) != 0 {
		t.Fatal(stderr.String())
	}
	if !strings.Contains(out.String(), `exact pin "favorite" overrides learned ranking`) || !strings.Contains(out.String(), "visits: 1") {
		t.Fatal(out.String())
	}
	pinsAfter, _ := os.ReadFile(store.PinsPath())
	after, _ = os.ReadFile(store.Path)
	if !bytes.Equal(before, after) || !bytes.Equal(pinsBefore, pinsAfter) {
		t.Fatal("explaining a pin changed private data")
	}
}

func TestExplainUnvisitedFavoriteAndMissingQuery(t *testing.T) {
	store := history.Store{Path: filepath.Join(t.TempDir(), "history.json")}
	if err := store.Pin("favorite", t.TempDir(), false); err != nil {
		t.Fatal(err)
	}
	var out, stderr bytes.Buffer
	app := App{Store: store, Out: &out, Err: &stderr}
	if app.Run([]string{"explain", "favorite"}) != 0 {
		t.Fatal(stderr.String())
	}
	if !strings.Contains(out.String(), "visits: 0; last visit: never recorded") {
		t.Fatal(out.String())
	}
	if _, err := os.Stat(store.Path); !os.IsNotExist(err) {
		t.Fatal("read-only explanation created history")
	}
	out.Reset()
	if app.Run([]string{"explain", "unknown"}) == 0 || out.Len() != 0 {
		t.Fatal("missing query should fail without a winner")
	}
}
