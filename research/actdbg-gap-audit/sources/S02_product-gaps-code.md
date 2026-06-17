---
id: S02
slug: product-gaps-code
title: Продуктовые дыры actdbg — анализ кода
channel: code-github (codebase analysis)
access: local
subquestion_ids: [ST2]
credibility: 5
recency: 5
bias: 1
total: 11
used: Y
---

## Ключевые ограничения (из кода)

### A. rerun --from N останавливается на первом uses:-шаге (КРИТИЧНО — 5/5)
- **Файл:** internal/snapshot/snapshot.go:407
- Логика: встречает uses: → `return nil` (полный выход, не skip)
- Нет skip с предупреждением и продолжением
- Реальный workflow: checkout (uses) → setup-node (uses) → cache (uses) → run: npm build
- Пользователь хочет `rerun --from 4` (упал build) — если шаги 1-3 = uses, rerun умирает
- README: "Coming next: re-running uses: steps." — обещано, не сделано

### B. ${{ }} выражения в run:-шагах выполняются буквально (4/5)
- **Файл:** internal/snapshot/snapshot.go:411
- Предупреждение есть, выполнение продолжается
- ${{ github.sha }}, ${{ secrets.TOKEN }}, ${{ matrix.os }} → в шелл буквально
- Большинство реальных workflow содержат хоть один ${{ }}

### C. Windows/macOS раннеры — запускается Linux контейнер без предупреждения (3/5)
- **Файл:** internal/fidelity/fidelity.go:123-126
- Фиксируется как ERR, но run не блокируется
- Пользователь получает случайный результат

### D. Reusable workflows — поддержка частичная (3/5)
- **Файл:** internal/fidelity/fidelity.go:142-143
- WARN, не ERR; secrets: inherit ломается

### E. Множественные job failures — записывается только первый (3/5)
- **Файл:** internal/enginerun/run.go:310 — `fail := tr.FirstFailure()`
- При параллельных jobs пользователь видит только один провал

### F. Снапшоты и inconsistent state после clean (2/5)
- JSON RunFile остаётся после clean; следующий `actdbg back N` → непонятная Docker-ошибка

### G. replay не работает с private repos без явного предупреждения (2/5)
- **Файл:** internal/replay/replay.go:53
- Ошибка только в момент фетча, не раньше

## act#2090 — summary
- Открыт 2023, остаётся открытым
- Люди хотят `act --shell` — интерактивный shell при падении
- Критичная боль: `docker exec` теряет env variables (actdbg это решает)
- Workaround: socat-reverse-shell в workflow (ужасный UX)
- Комментарий автора actdbg запощен 2026-06-14
