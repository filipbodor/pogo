package cli

import (
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/filipbodor/pogo/internal/history"
)

const help = `Pogo — learn your directories, then jump by name.

Usage:
  pogo add [PATH]        Record one visit (default: current directory)
  pogo query [WORDS...]  Print the highest-ranked matching directory
  pogo list [WORDS...]   Print all matching directories in rank order
  pogo remove PATH      Forget a directory, including a deleted one
  pogo prune            Forget directories that no longer exist
  pogo data             Print the private history file location
  pogo init zsh         Print Zsh integration (defines j and a visit hook)
  pogo --version        Print version
  pogo --help           Print this help

Use -- after a subcommand to pass words or paths beginning with a dash.
`

type App struct {
	Store   history.Store
	Out     io.Writer
	Err     io.Writer
	Version string
}

func (a App) Run(args []string) int {
	if err := a.run(args); err != nil {
		fmt.Fprintln(a.Err, "pogo:", err)
		return 1
	}
	return 0
}

func (a App) run(args []string) error {
	if len(args) == 0 || args[0] == "--help" || args[0] == "help" {
		fmt.Fprint(a.Out, help)
		return nil
	}
	command, args := args[0], args[1:]
	if len(args) > 0 && args[0] == "--" {
		args = args[1:]
	}
	switch command {
	case "--version":
		if len(args) != 0 {
			return errors.New("usage: pogo --version")
		}
		fmt.Fprintln(a.Out, "pogo", a.Version)
	case "init":
		if len(args) != 1 || args[0] != "zsh" {
			return errors.New("usage: pogo init zsh")
		}
		fmt.Fprint(a.Out, zsh)
	case "data":
		if len(args) != 0 {
			return errors.New("usage: pogo data")
		}
		fmt.Fprintln(a.Out, a.Store.Path)
	case "add":
		if len(args) > 1 {
			return errors.New("usage: pogo add [PATH]")
		}
		path := "."
		if len(args) == 1 {
			path = args[0]
		}
		return a.Store.Add(path, time.Now())
	case "remove":
		if len(args) != 1 {
			return errors.New("usage: pogo remove PATH")
		}
		return a.Store.Remove(args[0])
	case "prune":
		if len(args) != 0 {
			return errors.New("usage: pogo prune")
		}
		return a.Store.Prune()
	case "query", "list":
		for _, term := range args {
			if strings.ContainsAny(term, "\x00\r\n") {
				return errors.New("query words cannot contain NUL or newlines")
			}
		}
		db, err := a.Store.Read()
		if err != nil {
			return err
		}
		matches := history.Rank(db.Entries, args, time.Now())
		if command == "query" {
			if len(matches) == 0 {
				return errors.New("no matching directory; visit it first or use pogo add PATH")
			}
			_, err := fmt.Fprintln(a.Out, matches[0].Path)
			return err
		}
		for _, match := range matches {
			if _, err := fmt.Fprintln(a.Out, match.Path); err != nil {
				return err
			}
		}
	default:
		return fmt.Errorf("unknown command %q; use pogo --help", command)
	}
	return nil
}

// Run resolves storage only for commands that use it. Help and init remain usable
// even when a user's history location is not configured correctly.
func Run(args []string, out, stderr io.Writer, version string) int {
	app := App{Out: out, Err: stderr, Version: version}
	needsStore := len(args) > 0 && args[0] != "init" && args[0] != "--help" && args[0] != "help" && args[0] != "--version"
	if needsStore {
		path, err := history.DefaultPath()
		if err != nil {
			fmt.Fprintln(stderr, "pogo:", err)
			return 1
		}
		app.Store.Path = path
	}
	return app.Run(args)
}
