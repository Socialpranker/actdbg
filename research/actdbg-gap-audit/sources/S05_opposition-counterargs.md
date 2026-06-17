---
id: S05
slug: opposition-counterargs
title: Контр-аргументы против ценности actdbg (оппозиционный поиск)
channel: web-general + forum-discussion
access: public
subquestion_ids: [ST5]
credibility: 3
recency: 4
bias: 3
total: 10
used: Y
---

## Контр-аргументы (steel-man)

### CA1: "Есть action-tmate — зачем локальный debugger?" (3/5)
- action-tmate даёт SSH в реальный GitHub runner
- actdbg — локальный (не реальная среда)
- Rebuttal: tmate = постфактум на GitHub; actdbg = pre-push итерация за секунды
- Комбинируются, не конкурируют

### CA2: "Локальное != GitHub — зачем инвестировать?" (4/5 — САМЫЙ СИЛЬНЫЙ)
- Реальные различия: образы, OIDC, secrets, кеши
- Часть проблем воспроизводится только в реальной среде
- Rebuttal: act симулирует близко (те же Docker образы), ~90% проблем воспроизводятся
- actdbg честен об ограничениях через `check`; ROI положительный

### CA3: "GitHub улучшает DX — проблема уйдёт сама" (2/5)
- ACTIONS_STEP_DEBUG, debugger-action — GitHub инвестирует в remote debugging
- Rebuttal: remote debugging ≠ local debugging; ортогональные пути
- GitHub не планирует встраивать act/Docker-симуляцию

### CA4: "Act сам по себе достаточен" (3.5/5)
- act полнофункционален, документирован
- Зачем layer abstraction?
- Rebuttal: act — низкоуровневый; actdbg — DX слой (shell, env reconstruction, TUI)
- Как npm vs bash: оба нужны

### CA5: "Self-hosted runners решают проблему" (2.5/5)
- Self-hosted = production-like debugging
- Rebuttal: overkill для indie/mid teams; поддержка, maintenance, security
- actdbg демократизирует для 99% users без self-hosted infra

## Что не нашли
- Нет прямой критики actdbg (слишком низкая известность)
- Нет упоминания actdbg на Reddit/HN вне коммента автора в act#2090
- Нет альтернатив в нише "local debugger с shell + env reconstruction"
