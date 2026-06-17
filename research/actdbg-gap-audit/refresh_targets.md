---
slug: actdbg-gap-audit
last_research_date: 2026-06-17
next_refresh_suggested: 2026-09-17
---

# Refresh Targets — actdbg gap audit

## 1. Entities to track (fingerprint on each update)

| Entity | What to check | Where |
|---|---|---|
| nektos/act | Stars, latest version, open issues count, `--shell` FR status | github.com/nektos/act |
| action-tmate | Stars, last commit date, issues about TOS suspension | github.com/mxschmitt/action-tmate |
| actdbg | Stars, forks, open issues, Homebrew availability, GoReleaser setup | github.com/Socialpranker/actdbg |
| act#2090 | Status (open/closed), new comments, linked PRs | github.com/nektos/act/issues/2090 |
| lazygit | Stars (compare baseline 79.4k★ 2026-06-17) | github.com/jesseduffield/lazygit |
| GoReleaser | Breaking changes in new versions | github.com/goreleaser/goreleaser |
| `nektos/act v0.2.89` | Newer patch versions breaking actdbg, security advisories | go.mod / GitHub releases |

## 2. Numbers to refresh

| Metric | Baseline (2026-06-17) | Source |
|---|---|---|
| actdbg stars | ~check GitHub | github.com/Socialpranker/actdbg |
| act stars | 70.8k★ | github.com/nektos/act |
| action-tmate stars | 6k★ | github.com/mxschmitt/action-tmate |
| actdbg has Homebrew tap | NO | manual check |
| actdbg has binary releases | NO | manual check |
| actdbg has GitHub Topics | NO | manual check |
| CI platforms supporting SSH debug natively | CircleCI only | product pages |

## 3. Hypotheses to verify on update

| ID | Hypothesis | Current verdict | Watch for |
|---|---|---|---|
| H1 | actdbg невидим из-за distribution | CONFIRMED | Homebrew появился? binary releases появились? |
| H2 | rerun --from N сломан для реальных workflow | CONFIRMED (code) | Фикс snapshot.go:407 смержен? |
| H3 | Local != GitHub — главный контр-аргумент | PARTIALLY valid | Новые различия act vs GitHub? act обновился? |
| H4 | GitHub сам не добавит local sim | CONFIRMED | GitHub анонсировал local runner sim? |

## 4. Topic markers (search triggers)

Запросы для поиска нового на каждый update:

```
site:reddit.com "github actions" "local debug" after:LAST_DATE
site:news.ycombinator.com "github actions" debug shell after:LAST_DATE
"nektos/act" new release changelog after:LAST_DATE
"actdbg" -site:github.com/Socialpranker after:LAST_DATE
"github actions" debugger homebrew after:LAST_DATE
```

## 5. Code locations to re-audit

| File | What changed? | Risk if stale |
|---|---|---|
| `internal/snapshot/snapshot.go:407` | Фикс return nil → skip+warn? | Маркетинговая фича сломана |
| `internal/snapshot/snapshot.go:125` | Docker error молча игнорируется? | Диск заполняется без предупреждения |
| `main.go` | signal.NotifyContext добавлен? | Ctrl+C утечки |
| `go.mod` nektos/act version | Обновился? | Несовместимость + пропущенные фиксы |
| `doctor/` | Тесты появились? | Нулевое покрытие ключевого инструмента |

## 6. ICE-backlog status check

На каждый update проверить: что из NOW-списка сделано?

- [ ] GitHub Topics добавлены
- [ ] GoReleaser + binary releases настроены
- [ ] Homebrew tap создан (homebrew-actdbg)
- [ ] Ctrl+C signal handling исправлен
- [ ] snapshot.go:407 — return nil → skip+continue+warn
- [ ] Real demo.gif через vhs
