# Changelog

All notable changes to actdbg are documented here.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.5.0] — 2026-06-17 — rerun that actually works + Homebrew

### Added
- **Homebrew install** and prebuilt binaries (`linux`/`darwin` × `amd64`/`arm64`)
  via GoReleaser, published on every `v*` tag. `brew install Socialpranker/actdbg/actdbg`.
- Dependabot keeps `nektos/act` and CI actions up to date.

### Changed
- `rerun --from N` now **skips `uses:` actions and keeps going** instead of
  stopping dead on the first one. Their effect isn't reapplied (actdbg replays
  only `run:` steps), and it tells you exactly which were skipped — so fixing a
  later step no longer dead-ends on `actions/checkout`.
- `actdbg version` reports the released tag (injected at build time).

### Fixed
- **Graceful `Ctrl+C`.** A run is now cancelled cleanly instead of being killed
  mid-flight. The failed container is kept on purpose (that's the point), but
  actdbg no longer leaves orphaned containers and gigabytes of snapshot images
  behind silently — it points you at `actdbg clean`.
- **Honest snapshot failures.** When `docker commit` fails (disk full, rootless
  Docker/Podman), the timeline shows `snap ✗` instead of a false `✓`, and
  `actdbg back`/`rerun` explain that the step's snapshot is unavailable rather
  than failing with a cryptic "no snapshot for step N".

## [0.4.0] — lazygit-style TUI
### Added
- `actdbg ui` (and bare `actdbg` in a terminal): a lazygit-style TUI with jobs
  and steps on the left, the selected step's live log on the right, and the
  docker command log at the bottom.
- `strategy.matrix` picker before a matrixed run.
- Live command log mirrored in the TUI and written to `~/.actdbg/commands.log`.

## [0.3.0] — replay
### Added
- `actdbg replay <run-url>`: reproduce a real failed GitHub run locally from its
  URL — actdbg asks the GitHub API what failed, checks your checkout matches the
  run's commit (`--here` to override), and hands off to the debugger at the
  failure.

## [0.2.0] — time-travel
### Added
- Per-step snapshots (`docker commit` after each step, on by default).
- `actdbg back N`: a fresh container with the exact state right after step N.
- `actdbg rerun --from N`: re-run from a step instead of the whole job.
- `actdbg diff`: step x-ray — files each step touched and its `$GITHUB_ENV` delta.

## [0.1.0] — first release
### Added
- A debugger for GitHub Actions, built on `nektos/act` as a library.
- Stop at the first failed step; keep the job container alive.
- `actdbg shell`: drop into the failed step's container with its environment
  reconstructed (workflow/job/step `env:` chain + `$GITHUB_ENV`/`$GITHUB_PATH`).
- `actdbg check`: fidelity report of where a local run diverges from GitHub.
- `actdbg doctor`, `actdbg clean`.

[Unreleased]: https://github.com/Socialpranker/actdbg/compare/v0.5.0...HEAD
[0.5.0]: https://github.com/Socialpranker/actdbg/compare/v0.4.0...v0.5.0
[0.4.0]: https://github.com/Socialpranker/actdbg/compare/v0.3.0...v0.4.0
[0.3.0]: https://github.com/Socialpranker/actdbg/compare/v0.2.0...v0.3.0
[0.2.0]: https://github.com/Socialpranker/actdbg/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/Socialpranker/actdbg/releases/tag/v0.1.0
