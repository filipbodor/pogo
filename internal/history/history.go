// Package history stores private visit history and ranks directory matches.
package history

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type Entry struct {
	Path        string    `json:"path"`
	Visits      uint64    `json:"visits"`
	LastVisited time.Time `json:"last_visited"`
}

type Database struct {
	Version int     `json:"version"`
	Entries []Entry `json:"entries"`
}

type Store struct{ Path string }

// DefaultPath honors XDG_DATA_HOME on both supported operating systems.
func DefaultPath() (string, error) {
	base := os.Getenv("XDG_DATA_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		base = filepath.Join(home, ".local", "share")
	}
	if !filepath.IsAbs(base) {
		return "", errors.New("XDG_DATA_HOME must be an absolute path")
	}
	return filepath.Join(base, "pogo", "history.json"), nil
}

func Canonical(path string) (string, error) {
	if strings.ContainsAny(path, "\x00\r\n") {
		return "", errors.New("directory names containing NUL or newlines are unsupported")
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(absolute)
	if err == nil && strings.ContainsAny(resolved, "\x00\r\n") {
		return "", errors.New("resolved directory contains unsupported control characters")
	}
	return resolved, err
}

func Directory(path string) (string, error) {
	path, err := Canonical(path)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf("not a directory: %s", path)
	}
	return path, nil
}

func (s Store) Read() (Database, error) {
	data, err := os.ReadFile(s.Path)
	if errors.Is(err, os.ErrNotExist) {
		return Database{Version: 1}, nil
	}
	if err != nil {
		return Database{}, err
	}
	var db Database
	if err := json.Unmarshal(data, &db); err != nil {
		return Database{}, fmt.Errorf("invalid history; original left untouched: %w", err)
	}
	if db.Version != 1 {
		return Database{}, fmt.Errorf("unsupported history version %d", db.Version)
	}
	seen := make(map[string]bool)
	for _, entry := range db.Entries {
		if !filepath.IsAbs(entry.Path) || strings.ContainsAny(entry.Path, "\x00\r\n") ||
			entry.Visits == 0 || entry.LastVisited.IsZero() || seen[entry.Path] {
			return Database{}, errors.New("invalid history entry; original left untouched")
		}
		seen[entry.Path] = true
	}
	return db, nil
}

func (s Store) update(change func(*Database) error) error {
	directory := filepath.Dir(s.Path)
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return err
	}
	unlock, err := lock(s.Path + ".lock")
	if err != nil {
		return err
	}
	defer unlock()
	db, err := s.Read()
	if err != nil {
		return err
	}
	if err := change(&db); err != nil {
		return err
	}
	data, err := json.MarshalIndent(db, "", "  ")
	if err != nil {
		return err
	}
	return writeAtomic(s.Path, data)
}

func writeAtomic(path string, data []byte) error {
	f, err := os.CreateTemp(filepath.Dir(path), ".pogo-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err := f.Write(append(data, '\n')); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}

func (s Store) Add(path string, now time.Time) error {
	path, err := Directory(path)
	if err != nil {
		return err
	}
	return s.update(func(db *Database) error {
		for i := range db.Entries {
			if db.Entries[i].Path == path {
				if db.Entries[i].Visits < math.MaxUint64 {
					db.Entries[i].Visits++
				}
				db.Entries[i].LastVisited = now.UTC()
				return nil
			}
		}
		db.Entries = append(db.Entries, Entry{Path: path, Visits: 1, LastVisited: now.UTC()})
		return nil
	})
}

// Remove accepts stale paths, which can no longer be resolved through symlinks.
func (s Store) Remove(path string) error {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	// Resolve the nearest surviving parent too: on macOS /var aliases /private/var,
	// even when the final directory no longer exists.
	parent := absolute
	var suffix []string
	for {
		resolved, err := filepath.EvalSymlinks(parent)
		if err == nil {
			for i := len(suffix) - 1; i >= 0; i-- {
				resolved = filepath.Join(resolved, suffix[i])
			}
			absolute = resolved
			break
		}
		if !errors.Is(err, os.ErrNotExist) || filepath.Dir(parent) == parent {
			break
		}
		suffix = append(suffix, filepath.Base(parent))
		parent = filepath.Dir(parent)
	}
	return s.update(func(db *Database) error {
		entries := db.Entries[:0]
		for _, entry := range db.Entries {
			if entry.Path != absolute {
				entries = append(entries, entry)
			}
		}
		db.Entries = entries
		return nil
	})
}

func (s Store) Prune() error {
	return s.update(func(db *Database) error {
		entries := db.Entries[:0]
		for _, entry := range db.Entries {
			info, err := os.Stat(entry.Path)
			if (err == nil && info.IsDir()) || (err != nil && !errors.Is(err, os.ErrNotExist)) {
				entries = append(entries, entry)
			}
		}
		db.Entries = entries
		return nil
	})
}

type Match struct {
	Entry
	Score         float64
	PinName       string
	Frequency     float64
	AgeDays       float64
	RecencyFactor float64
	BasenameBoost float64
}

func scoreEntry(entry Entry, terms []string, now time.Time) Match {
	ageDays := 0.0
	if !entry.LastVisited.IsZero() {
		ageDays = math.Max(0, now.Sub(entry.LastVisited).Hours()/24)
	}
	frequency := math.Log1p(float64(entry.Visits))
	recency := 1 / (1 + ageDays/7)
	boost := 1.0
	if len(terms) > 0 && strings.Contains(strings.ToLower(filepath.Base(entry.Path)), strings.ToLower(terms[len(terms)-1])) {
		boost = 2
	}
	return Match{Entry: entry, Score: frequency * recency * boost,
		Frequency: frequency, AgeDays: ageDays, RecencyFactor: recency, BasenameBoost: boost}
}

// Rank uses ordered, case-insensitive literal substrings, not regex or shell code.
func Rank(entries []Entry, terms []string, now time.Time) []Match {
	var matches []Match
	for _, entry := range entries {
		remaining := strings.ToLower(entry.Path)
		matched := true
		for _, term := range terms {
			term = strings.ToLower(term)
			index := strings.Index(remaining, term)
			if index < 0 {
				matched = false
				break
			}
			remaining = remaining[index+len(term):]
		}
		if !matched {
			continue
		}
		info, err := os.Stat(entry.Path)
		if err != nil || !info.IsDir() {
			continue
		}
		matches = append(matches, scoreEntry(entry, terms, now))
	}
	sort.Slice(matches, func(i, j int) bool {
		if matches[i].Score == matches[j].Score {
			return matches[i].Path < matches[j].Path
		}
		return matches[i].Score > matches[j].Score
	})
	return matches
}
