# Contributing to actdbg

Thanks for taking a look. actdbg is a small Go CLI built on
[nektos/act](https://github.com/nektos/act) as a library. Bug reports, fidelity
rules, and focused PRs are all welcome.

## Prerequisites

- **Go 1.25+** (see `go.mod`)
- **Docker** running — actdbg drives real containers, and so do its end-to-end
  tests. Run `actdbg doctor` (or `go run . doctor`) to sanity-check your setup.

## Build, test, run

```bash
go build -o actdbg .     # build the binary
go vet ./...             # static checks
go test ./...            # unit tests (no Docker needed)
go run . run             # run actdbg against ./.github/workflows
```

The unit tests are deliberately Docker-free: pure logic lives in small functions
(env parsing, snapshot bookkeeping, plan inspection) that are tested in
isolation. The full container path is exercised by the `e2e` job in
`.github/workflows/ci.yml` — read it to see what end-to-end behavior is pinned.

## Project layout

Everything lives under `internal/`:

| package | responsibility |
|---|---|
| `enginerun` | drives act's runner: plan → execute → stop at failure → hand off |
| `snapshot` | time-travel: `docker commit` per step, `back` / `rerun` / `diff` |
| `shellenv` | reconstructs the step's environment and opens the debug shell |
| `replay` | fetches a failed GitHub run and maps it to a local run |
| `fidelity` | the `check` report: where a local run diverges from GitHub |
| `timeline` | step-by-step event tracking and failure detection |
| `ui` | the lazygit-style TUI (bubbletea / lipgloss) |
| `cmdlog` | logs every docker command actdbg runs (`~/.actdbg/commands.log`) |
| `doctor` | Docker / image / arch diagnostics |
| `state` | persists the last stopped step so `actdbg shell` works later |

`main.go` is the CLI surface (flag parsing + command dispatch).

## Conventions

- **Honesty over magic.** actdbg's value is being truthful about where local ≠
  GitHub. Don't paper over a divergence with a silent fallback — surface it
  (a `fidelity` rule, a printed warning, a distinct error). A silent "✓" that
  isn't true is a bug, not a convenience.
- **Keep the corpse.** A failed run leaves its container alive on purpose. Don't
  add auto-cleanup that removes containers users may want to inspect; cleanup is
  an explicit `actdbg clean`.
- **Conventional commits:** `feat:`, `fix:`, `refactor:`, `docs:`, `test:`,
  `chore:` (with an optional scope, e.g. `fix(rerun): …`).
- New behavior that can be tested without Docker should come with a unit test;
  new end-to-end behavior should be pinned in the CI `e2e` job.

## Submitting a PR

1. Open an issue first for anything non-trivial, so we agree on the approach.
2. Keep PRs focused — one logical change. `go vet` and `go test ./...` must pass.
3. Update `CHANGELOG.md` (the `[Unreleased]` section) and, if user-facing,
   `README.md`.

## License

By contributing you agree your work is licensed under the project's MIT license.
