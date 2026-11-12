# Pogo

**Bounce back to places you know.**

Pogo learns the directories you visit, then lets you jump to them by name:

```sh
j client
j web client
j "my project"
```

A small, independent Go directory jumper.
The first version supports macOS, Linux, and Zsh. It uses the Go standard library
and produces a standalone executable.

## Install

Download the archive for your machine from [Releases](https://github.com/filipbodor/pogo/releases/latest).
`darwin` means macOS, `arm64` means Apple Silicon or Linux ARM, and `amd64` means
Intel/AMD. Download `checksums.txt` into the same directory and verify the archive
before extracting it:

```sh
shasum -a 256 --ignore-missing -c checksums.txt
tar -xzf pogo_v0.1.0_darwin_arm64.tar.gz  # Use your downloaded filename
mkdir -p ~/.local/bin
install -m 755 pogo ~/.local/bin/pogo
```

Your downloaded archive should report `OK`. On Linux, you can use
`sha256sum --ignore-missing -c checksums.txt` instead. No Go installation or other
language runtime is required to run the release binary.

With Go 1.26 or newer, you can also install from source:

```sh
go install github.com/filipbodor/pogo/cmd/pogo@latest
```

Make sure `$(go env GOPATH)/bin` is on your PATH. To build from a checkout and
install into a local binary directory instead:

```sh
go build -o build/pogo ./cmd/pogo
mkdir -p ~/.local/bin
install -m 755 build/pogo ~/.local/bin/pogo
```

Ensure `~/.local/bin` is on your PATH, then add this once near the end of `.zshrc`,
after any framework or other shell integrations:

```sh
eval "$(pogo init zsh)"
```

Open a new terminal or run the same line in the current Zsh session. This defines
`j` and records visits when the working directory changes. Existing directory
change hooks are preserved. Re-sourcing the integration does not duplicate hooks.
The names `j` and `_pogo_record` are reserved by the integration; it replaces an
existing function with either name. Prompt redraws do not count as visits.

## Everyday use

```sh
j client                    # Best learned match
j web client                # Ordered fragments anywhere in the path
j "project with spaces"
j ~/Projects/new-app        # Existing directories work directly and are learned
j -                         # Previous directory, like cd -
j                           # Home directory
```

Pogo starts with an empty history. Visit directories normally to teach it, or
record them explicitly:

```sh
pogo add ~/Projects/app
pogo list                   # Ranked paths, one per line
pogo list client            # Filter the list
pogo query client           # Print the best path without changing directories
pogo remove ~/Projects/app  # Forget a path; also works after deleting it
pogo prune                  # Remove entries that are no longer directories
pogo data                   # Locate the private history file
pogo --help
```

For paths or words beginning with a dash, use `--` after the subcommand:
`pogo add -- ./-example`. Quoted directory names are passed as data; Pogo does not
evaluate them as shell commands. Paths containing NUL or newline characters are
unsupported. Symlinks are resolved when recording visits, so aliases share history.
Queries skip inaccessible or missing directories.

## Named favorites

```sh
pogo pin work ~/Projects/app
j work
pogo pins
pogo pin --replace work ~/Projects/another-app
pogo unpin work
```

Names are case-insensitive and may contain letters, digits, dots, underscores,
and hyphens, starting with a letter or digit. An exact name wins over learned
rankings and a same-named local directory. Use an explicit path such as `j ./work`
to visit the local directory instead. Multiple search words continue to use
normal learned matching. `j` still means home and `j -` still means the previous
directory. Open a new terminal or re-run `eval "$(pogo init zsh)"` after upgrading
so an existing shell gets the new pin precedence.

Replacing a pin requires `--replace`; re-pinning the same path is harmless.
Missing pinned directories produce an error and never silently fall back to a
different destination. `prune` and `remove` manage learned visits; explicit pins
remain until `unpin`. Pinning and unpinning do not manufacture or erase visits.

Favorites are stored in private `pins.json` beside `history.json`, with their own
lock and atomic writes. The history format is unchanged and older versions cannot
overwrite favorites. `pogo query --pin NAME` resolves only an exact favorite;
exit status 2 means it is not configured, while status 1 reports an error.

## Matching and ranking

Words are case-insensitive literal substrings that must occur in order in the
full path. No regex syntax or fuzzy edit-distance matching is involved.

Each matching directory gets this score:

```text
log(1 + visits) / (1 + days_since_last_visit / 7)
```

The score doubles when the final search word matches the directory's basename.
Frequency helps familiar directories; recency helps recent visits. Ties are
ordered by path for predictable results. Change `Rank` in
`internal/history/history.go` to experiment with your own ranking.

## Explain a jump

```sh
pogo explain client
pogo explain work
```

This uses the same resolver as `pogo query`, showing the winner, whether an exact
pin overrides ranking, and each matching path's visits, last visit, age, frequency
weight, recency factor, basename multiplier, and final score. It is read-only and
records no visits. An unvisited favorite is labeled as having no recorded visits.
For `j ./directory` and other direct filesystem paths, normal `cd` rules apply;
`explain` describes pin and learned-query resolution.

## Local history

The default database is `~/.local/share/pogo/history.json`. An absolute
`XDG_DATA_HOME` changes its base directory. Pogo creates its data directory with
mode `0700` and history/lock files with mode `0600`. Keep the history out of Git:
it contains private paths and visit times.

Pogo makes no network requests, scans no directory trees, and reads no project
configuration. Writes are locked across processes and committed through an atomic
file replacement. Lock acquisition times out after one second. Invalid history
or an unknown schema version produces an error without overwriting the original.
The directory-change hook suppresses errors so normal `cd` commands still work;
run `pogo list` or `pogo add` directly to diagnose a storage problem.

To stop learning directories, remove the init line and open a new shell. To clear
history, stop the integration first and delete the file printed by `pogo data`.
Other shells, interactive selection, exclusions, and history migration are useful
future extensions; the first version focuses on the basic directory-jumping loop.

## Development

```sh
gofmt -w cmd internal
go vet ./...
go test -race ./...
go build -o build/pogo ./cmd/pogo
./build/pogo --version
./scripts/release.sh v0.1.0  # Build macOS/Linux archives and SHA-256 checksums
```

- `internal/history`: storage, concurrent writes, matching, and ranking.
- `internal/cli`: commands and the emitted Zsh integration.
- `cmd/pogo`: executable entry point.

Tests use temporary directories and include separate-process writes, corrupt data,
private permissions, real Zsh jumps, existing hooks, and shell metacharacters in
directory names. Install Zsh to run the shell integration test. CI tests macOS and
Linux with Go 1.26 and 1.27. Linux CI builds a pinned upstream Zsh 5.9 source
revision using the runner's build tools, avoiding package-mirror availability.
Missing Zsh fails CI instead of silently skipping the integration test.

[MIT licensed](LICENSE). Focused issues and pull requests are welcome.
