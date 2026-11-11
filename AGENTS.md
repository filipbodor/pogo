# Pogo

- Support macOS and Linux with Go 1.26+; keep the runtime dependency-free.
- Directory history is private user data. Never commit real paths or history.
- Treat directory names as data, never shell source. Use temporary paths in tests.
- Preserve history on malformed data or write failures. Lock read-modify-write operations and replace files atomically.
- Shell integration must preserve existing hooks and avoid counting prompt redraws as visits.
- Run gofmt, go vet ./..., and go test -race ./... before committing code changes.
