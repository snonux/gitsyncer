# Repository Guidelines

## Project Structure & Module Organization
- `cmd/gitsyncer/`: CLI entrypoint (`main.go`).
- `internal/`: private packages — `cli/` (flags/handlers), `sync/` (core git ops), `config/`, `github/`, `codeberg/`, `release/`, `showcase/`, `state/`, `version/`, `cmd/` (cobra wiring).
- `test/`: integration tests (`run_integration_tests.sh`, `test_*.sh`) and local configs.
- `doc/`: architecture, API, and development docs. `dist/`: build artifacts.

## Build, Test, and Development Commands
- Build (current platform): `task build` or `go build -o gitsyncer ./cmd/gitsyncer`.
- Cross-compile: `task build-all` (linux/darwin/windows variants also available).
- Run binary: `./gitsyncer --help` or `task run -- --version`.
- Unit tests (when present): `task test` or `go test ./...`.
- Integration tests: `cd test && ./run_integration_tests.sh`.
- Format/lint: `task fmt`, `task vet`, `task lint` (golangci-lint).
- Tidy modules: `task mod-tidy`.

## Coding Style & Naming Conventions
- Go formatting: enforce `gofmt`/`go fmt`; prefer `goimports` in your editor.
- Naming: exported identifiers in CamelCase; acronyms ALLCAPS (ID, URL, API).
- Errors: wrap with context using `%w` (`fmt.Errorf("context: %w", err)`).
- Interfaces: accept interfaces, return concrete types where practical.

## Testing Guidelines
- Frameworks: standard `testing` for unit tests; integration via `test/*.sh`.
- Add unit tests alongside packages (e.g., `internal/sync/sync_test.go`).
- Run full suite locally: `go test ./...` then `cd test && ./run_integration_tests.sh`.
- Prefer meaningful test names: `TestFeature_Behavior` and `test_*.sh` for scripts.

## Commit & Pull Request Guidelines
- Commits: Conventional Commits format — `type(scope): summary`.
  - Examples: `feat(sync): add bidirectional mode`, `fix(config): validate organizations`.
- PRs: clear description, linked issues, testing notes (unit + integration), and updated docs (`doc/` or `README.md`) when user-visible.
- Keep changes focused; run `task fmt vet lint` before submitting.

## Security & Configuration Tips
- Configuration lives at `~/.config/gitsyncer/config.json` (see `gitsyncer.example.json`). Do not commit secrets.
- Use `--config` and `--work-dir` to target isolated test setups.
- Debugging: set `GITSYNCER_DEBUG=1` to enable extra logs where supported.
