---
slug: actdbg-gap-audit
created: 2026-06-17
updated: 2026-06-17
depth: medium
report_type: custom
blocks: [tldr, scope, swot, data-table, persona, feature-matrix, white-spaces, risk-register, ranked-list, actionable-next-steps, counter-arguments, open-questions, next-research, map-of-sources, metadata]
status: completed
version: initial
parent: null
time_box_target: ~3 hours
time_box_hard: 5 hours
---

# Plan — actdbg gap audit: что не хватает (adoption + product + tech)

## 0. User context

- **Кто:** Ivan Teresenko, соло-разработчик actdbg
- **Зачем:** понять что делать дальше с наибольшей отдачей перед выходом «в люди» (open-source traction, первые реальные пользователи)
- **Baseline:** автор знает проект глубоко, нужен взгляд снаружи — как воспринимается со стороны, что мешает adoption, что ждут пользователи
- **Использование отчёта:** приоритизированный backlog (now/next/later) + аналитический обзор для принятия решения что делать следующим
- **Constraints:** соло, ограниченное время; enterprise/SaaS вещи не предлагать

## 1. Time-box

- **Target:** ~3 часа
- **Hard deadline:** 5 часов от старта
- **Если превысили:** синтезировать с тем что есть

---

# SCOPE

## 2. Главный вопрос

Что конкретно мешает actdbg получить traction среди разработчиков GitHub Actions — по осям adoption/discoverability, product completeness, и tech quality — и в каком порядке это исправлять соло-разработчику с ограниченным временем?

## 3. Решение, которое поддерживает

- **Что решаем:** приоритеты для следующих 1-2 месяцев работы над проектом
- **Что меняется от ответа:** один из трёх путей: (A) сначала контент+дистрибуция, (B) сначала фичи, (C) сначала техкачество
- **Решение конкретное** — backlog с ICE-скорингом

## 4. Acceptance criteria

- [ ] `2026-06-17_custom.md` содержит все required блоки
- [ ] Каждая гипотеза H1-H4 получила статус (confirmed / contradicted / partial / insufficient)
- [ ] SWOT по actdbg заполнен конкретными фактами (не абстракциями)
- [ ] Feature-matrix: actdbg vs act vs action-tmate vs CircleCI-SSH по ≥8 осям
- [ ] Backlog: ≥10 конкретных gap'ов с ICE и волнами (now/next/later)
- [ ] Actionable next steps: топ-3 с первым конкретным шагом каждый
- [ ] Counter-arguments ≥2
- [ ] Adversarial pass пройден
- [ ] Все sources/NN.md с channel + access полями

## 5. Discovered existing

- Существующих ресёрчей в проекте нет (папка research/ не существовала)
- Memory: env-fix и pull-warning смержены в main; реальный demo.gif — единственный открытый пункт на 2026-06-14
- Memory: коммент в act#2090 запощен 2026-06-14
- CLAUDE.md не упоминает research-тему
- **Решение:** initial research

## 6. Глоссарий

- **traction** — признаки интереса сообщества: звёзды, issues от внешних, mention в соцсетях
- **adoption-барьер** — что мешает человеку установить и использовать в первый раз
- **fidelity gap** — расхождение между локальным запуском и реальным GitHub
- **time-travel** — фича actdbg: снапшоты контейнера после каждого шага (back/rerun/diff)
- **uses:-step** — шаг workflow вида `uses: actions/checkout@v4` (внешний action), в отличие от `run:` (shell-команды)
- **ICE** — Impact × Confidence ÷ Effort (приоритизационный фреймворк)
- **pet-с-амбицией** — стадия проекта: личный проект без внешних пользователей, но с целью выйти в публичный open-source

---

# STRUCTURE

## 7. Жанр

**custom** — гибрид «карта дыр» + «по показателям» + «приоритизация». Ни один стандартный жанр не покрывает все три оси одновременно.

## 8. Блоки

| Порядок | Блок [ID] | Зачем |
|---|---|---|
| 1 | tldr [F1] | Быстрый вывод для принятия решения |
| 2 | scope [F3] | Границы аудита |
| 3 | persona [P1] | Кто целевой пользователь actdbg — нужно для оценки gap'ов |
| 4 | swot [A5] | Структурированная картина сильных/слабых сторон и возможностей |
| 5 | data-table [A1] | actdbg vs конкуренты по ключевым осям |
| 6 | feature-matrix [C10] | Фичи × инструменты (есть/нет/частично) |
| 7 | white-spaces [M5] | Где пустые ниши — что actdbg мог бы закрыть |
| 8 | risk-register [A6] | Технические и продуктовые риски |
| 9 | ranked-list [M7] | Приоритизированный backlog по волнам + ICE |
| 10 | actionable-next-steps [Z4] | Топ-3 первых шага с конкретными действиями |
| 11 | counter-arguments [Z1] | Steel-man против наших выводов |
| 12 | open-questions [Z2] | Что не закрыто |
| 13 | next-research [Z3] | Следующие ресёрчи |
| 14 | map-of-sources [Z5] | Карта источников |
| 15 | metadata [F5] | Footer |

## 9. Гипотезы

- **H1:** Главный adoption-барьер — дистрибуция и discoverability, а не фичи (нет Homebrew, нет gotcha-demo, нет channels — в отличие от lazygit/k9s/dive)
- **H2:** Функциональная дыра №1 — невозможность реранить `uses:`-шаги; это блокер для большинства реальных workflow
- **H3:** Техкачество достаточное для pet, но утечки контейнеров/диска и обработка ошибок Docker станут первой болью при реальных юзерах
- **H4:** Позиционирование «debugger, not emulator» сильное, но требует лучшей подачи — большинство не прочитают "Honesty" секцию до разочарования

## 10. Risk register (pre-mortem)

| ID | Risk | Prob | Impact | Mitigation |
|---|---|---|---|---|
| R1 | Мало данных о реальных пользователях (нет issues, нет reviews) | high | medium | Опираться на issues nektos/act + форумы как proxy |
| R2 | Конкуренты (act, tmate) плохо задокументированы с UX-стороны | medium | low | Использовать GitHub stars/issues как indirect signal |
| R3 | Субъективность ICE без реальных данных | high | medium | Признать честно, пометить как «авторская оценка» |

---

# EXECUTION

## 11. Подтемы ↔ Блоки mapping

| Подтема | Под блоки | Кому |
|---|---|---|
| ST1: Дистрибуция и discoverability конкурентов | data-table, feature-matrix, H1 | Explore #1 (haiku) |
| ST2: Продуктовые дыры — uses:, matrix, services, composite | feature-matrix, white-spaces, H2 | Explore #2 (sonnet — код) |
| ST3: Пользователи GitHub Actions + что они хотят | persona, data-table, swot | Explore #3 (haiku) |
| ST4: Техкачество — тесты, ошибки Docker, ARM, снапшоты | risk-register, swot, H3 | Explore #4 (sonnet — код) |
| ST5: Позиционирование и подача в README | swot, H4, counter-args | main thread |

## 12. Information sourcing strategy

### ST1: Дистрибуция и discoverability конкурентов
**Под блоки:** data-table, feature-matrix, H1
- **Primary:** code-github (GitHub repos: lazygit, k9s, dive, act, nektos/act) — звёзды, README-структура, install-секции, Topics
- **Secondary:** web-general — «go cli tool homebrew install best practices», «developer tool launch HN»
- **Fallback:** forum-discussion (HN, Reddit /r/devops, /r/golang)
- **Capabilities check:** ✅ WebSearch + WebFetch, ✅ GitHub API public (без токена)

### ST2: Продуктовые дыры (код)
**Под блоки:** feature-matrix, white-spaces, H2
- **Primary:** code-github — читать actdbg/enginerun/, replay/, snapshot/ на предмет TODO/ограничений; issues nektos/act #2090
- **Secondary:** web-general — «github actions uses step debug», «act uses step support»
- **Capabilities check:** ✅ Read + Bash grep в репозитории

### ST3: Пользователи + что хотят
**Под блоки:** persona, swot (Opportunities), H1
- **Primary:** forum-discussion — Reddit /r/devops, /r/github, GitHub Actions community; HN issues с тегом «github actions»
- **Secondary:** web-general — «github actions debug locally», «act alternative», HN «debug ci locally»
- **Capabilities check:** ✅ WebSearch + WebFetch

### ST4: Техкачество (код)
**Под блоки:** risk-register, swot (Weaknesses), H3
- **Primary:** code-github — читать тесты, обработку ошибок в enginerun/, doctor/, clean/
- **Secondary:** web-general — «act docker leak containers», «go docker exec error handling»
- **Capabilities check:** ✅ Read + Bash grep

## 13. Critical opposition queries

- «actdbg alternatives»
- «act --shell feature not needed»
- «why not just use act»
- «github actions debug locally criticism»
- «tmate vs actdbg»
- «local ci debugging problems»

## 14. Stop-criteria

- H1-H4 покрыты ≥3 источниками каждая
- Покрыты ≥4 типа источников (код, форум, web-general, GitHub)
- Целевой поиск оппозиции выполнен
- НЕТ новой информации в последних 3-5 источниках

---

# TRACKING

## 15. Notes during research

- 2026-06-17: initial research, нет существующих ресёрчей в проекте
- Memory: единственный открытый пункт из прошлой сессии — реальный demo.gif
- Memory: коммент в act#2090 запощен, что важно для ST3 (видимость в сообществе)

## 16. Update changelog

N/A — initial.

---

## Slug

actdbg-gap-audit
