---
id: S04
slug: tech-quality-code
title: Техкачество actdbg — тесты, ошибки Docker, кросс-платформа
channel: code-github (codebase analysis)
access: local
subquestion_ids: [ST4]
credibility: 5
recency: 5
bias: 1
total: 11
used: Y
---

## Тестовое покрытие (таблица)

| Пакет | Тесты | Оценка полноты (1-5) | Что не покрыто | Критичность |
|---|---|---|---|---|
| enginerun/run | ✅ | 1/5 | RunWithTracker, сигналы, Docker-ошибки | КРИТИЧЕСКАЯ |
| enginerun/pull | ✅ | 3/5 | warnIfPullNeeded | средняя |
| snapshot | ✅ | 2/5 | Restore, Rerun, CleanArtifacts | КРИТИЧЕСКАЯ |
| shellenv | ✅ | 4/5 | Enter(), ExecShell | высокая |
| timeline | ✅ | 3/5 | Fire() hook, OnLine callback | высокая |
| cmdlog | ✅ | 4/5 | race под нагрузкой | низкая |
| fidelity | ✅ | 4/5 | network/service секции | средняя |
| replay | ✅ | 1/5 | основная логика fetch+parse | высокая |
| ui | ✅ | 1/5 | TUI state machine, RunWithTracker путь | высокая |
| doctor | ❌ | — | всё | средняя |
| state | ❌ | — | Save/Load | средняя |

## Топ-3 технических риска

### Риск 1: Нет обработки сигналов (КРИТИЧНО)
- main.go: нет `signal.NotifyContext`
- enginerun/run.go:308: `context.Background()` без cancel
- Ctrl+C → process убивается, act-контейнер и снапшоты живут
- Накопление GB образов `actdbg/snap:*` при активном использовании
- Первый issue на GitHub: "my disk is full after using actdbg"
- Fix: 2 часа — `signal.NotifyContext` + `defer Clean()` в main()

### Риск 2: Shell injection в snapshot.Rerun (СРЕДНИЙ)
- internal/snapshot/snapshot.go:450
- Скрипт строится конкатенацией без экранирования heredoc-границы
- `run: |` с EOF-строкой может вырваться из скрипта
- Для инструмента читающего чужие workflow — риск

### Риск 3: Docker-ошибки в snapshot.OnStepResult молча игнорируются
- internal/snapshot/snapshot.go:125 — `_ = cmdlog.Docker("commit", ...).Run()`
- snapshot.go:181 — `_ = os.WriteFile(...)`
- Если docker commit падает (disk full, Podman, rootless) — TUI показывает "✓"
- `actdbg back 3` → "no snapshot for step 3" без объяснения
- При разных Docker-конфигурациях (Podman, Docker Desktop arm64) — частый сценарий

## Зависимость от nektos/act
- go.mod: `github.com/nektos/act v0.2.89`
- Act активно развивается, патч-версии регулярны
- Отставание на несколько патч-версий: баги в container.options, GITHUB_ENV формат
- Стратегии нет — нет Dependabot, нет update-скрипта

## Что CI не покрывает
- doctor — нет тестов
- state.Save/Load — нет тестов
- replay — полная логика не тестируется
- Платформы: только ubuntu-latest (нет macOS, нет arm64)
- actdbg back N при прерванном run
- Одновременные запуски
- Docker daemon упал в середине run
- --arch linux/amd64 путь
