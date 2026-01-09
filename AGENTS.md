# Repository Guidelines

## Project Structure & Module Organization
- `cmd/storage-tui/main.go` is the entry point for the TUI binary.
- `internal/app/` owns layout, navigation, and selection logic.
- `internal/azure/` defines the provider interface and mock data used by the UI.
- `go.mod` / `go.sum` manage Go module dependencies.
- `storage-tui` (if present) is a local build artifact; it can be regenerated with `go build`.

## Build, Test, and Development Commands
- `go run ./cmd/storage-tui` runs the TUI with the current mock data.
- `go build -o storage-tui ./cmd/storage-tui` produces a local binary at `./storage-tui`.
- `go test ./...` runs all Go tests (currently none are checked in).
- `go vet ./...` runs basic static analysis before pushing changes.

## Coding Style & Naming Conventions
- Follow standard Go formatting: run `gofmt -w` on modified files.
- Keep package names lowercase and short (`app`, `azure`).
- Use `*_test.go` for tests and prefer table-driven test names like `TestProvider_ListAccounts`.
- Keep UI logic in `internal/app` and provider logic in `internal/azure` to avoid mixed concerns.

## Testing Guidelines
- No formal coverage requirement yet; add unit tests as behavior stabilizes.
- Place tests next to the code they cover (same directory), using `*_test.go` files.
- Run `go test ./...` before opening a PR that changes behavior.

## Commit & Pull Request Guidelines
- Commit history is short and informal; use concise, imperative messages like
  "Add refresh key handling" or "Update README".
- PRs should include a brief summary, a short list of changes, and the command
  output used to validate (for example, `go test ./...`).
- For UI changes, include a screenshot or short GIF when possible.
