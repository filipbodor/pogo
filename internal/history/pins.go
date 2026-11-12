package history

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

var pinPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]*$`)

type pinDatabase struct {
	Version int               `json:"version"`
	Pins    map[string]string `json:"pins"`
}

func pinName(name string) (string, error) {
	name = strings.ToLower(name)
	if !pinPattern.MatchString(name) {
		return "", errors.New("pin names must start with a letter or digit and contain only ASCII letters, digits, dots, underscores, or hyphens")
	}
	return name, nil
}

// PinsPath keeps explicit favorites independent of learned history and v0.1 writes.
func (s Store) PinsPath() string { return filepath.Join(filepath.Dir(s.Path), "pins.json") }

func (s Store) Pins() (map[string]string, error) {
	data, err := os.ReadFile(s.PinsPath())
	if errors.Is(err, os.ErrNotExist) {
		return map[string]string{}, nil
	}
	if err != nil {
		return nil, err
	}
	var db pinDatabase
	if err := json.Unmarshal(data, &db); err != nil {
		return nil, fmt.Errorf("invalid pins; original left untouched: %w", err)
	}
	if db.Version != 1 {
		return nil, fmt.Errorf("unsupported pins version %d", db.Version)
	}
	for name, path := range db.Pins {
		normalized, err := pinName(name)
		if err != nil || normalized != name || !filepath.IsAbs(path) || strings.ContainsAny(path, "\x00\r\n") {
			return nil, errors.New("invalid pin entry; original left untouched")
		}
	}
	if db.Pins == nil {
		db.Pins = map[string]string{}
	}
	return db.Pins, nil
}

func (s Store) updatePins(change func(map[string]string) error) error {
	if err := os.MkdirAll(filepath.Dir(s.PinsPath()), 0o700); err != nil {
		return err
	}
	unlock, err := lock(s.PinsPath() + ".lock")
	if err != nil {
		return err
	}
	defer unlock()
	pins, err := s.Pins()
	if err != nil {
		return err
	}
	if err := change(pins); err != nil {
		return err
	}
	data, err := json.MarshalIndent(pinDatabase{Version: 1, Pins: pins}, "", "  ")
	if err != nil {
		return err
	}
	return writeAtomic(s.PinsPath(), data)
}

func (s Store) Pin(name, path string, replace bool) error {
	name, err := pinName(name)
	if err != nil {
		return err
	}
	path, err = Directory(path)
	if err != nil {
		return err
	}
	return s.updatePins(func(pins map[string]string) error {
		if previous, exists := pins[name]; exists && previous != path && !replace {
			return fmt.Errorf("pin %q already exists; use pogo pin --replace NAME PATH to change it", name)
		}
		pins[name] = path
		return nil
	})
}

func (s Store) Unpin(name string) error {
	name, err := pinName(name)
	if err != nil {
		return err
	}
	return s.updatePins(func(pins map[string]string) error {
		if _, exists := pins[name]; !exists {
			return fmt.Errorf("unknown pin %q", name)
		}
		delete(pins, name)
		return nil
	})
}

func (s Store) Pinned(name string) (Match, bool, error) {
	pins, err := s.Pins()
	if err != nil {
		return Match{}, false, err
	}
	name = strings.ToLower(name)
	path, found := pins[name]
	if !found {
		return Match{}, false, nil
	}
	resolved, err := Directory(path)
	if err != nil {
		return Match{}, true, fmt.Errorf("pin %q points to an unavailable directory (%s): %w", name, path, err)
	}
	return Match{Entry: Entry{Path: resolved}, PinName: name}, true, nil
}

// Resolve puts an exact, case-insensitive pin ahead of all learned matches.
func (s Store) Resolve(terms []string, now time.Time) ([]Match, error) {
	db, err := s.Read()
	if err != nil {
		return nil, err
	}
	matches := Rank(db.Entries, terms, now)
	if len(terms) != 1 {
		return matches, nil
	}
	pinned, found, err := s.Pinned(terms[0])
	if err != nil {
		return nil, err
	}
	if !found {
		return matches, nil
	}
	for _, entry := range db.Entries {
		if entry.Path == pinned.Path {
			pinned.Entry = entry
			break
		}
	}
	result := []Match{pinned}
	for _, match := range matches {
		if match.Path != pinned.Path {
			result = append(result, match)
		}
	}
	return result, nil
}
