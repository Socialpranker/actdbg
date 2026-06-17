---
id: S03
slug: user-pain-points
title: Боли разработчиков при дебаге GitHub Actions (форумы, Reddit, HN)
channel: forum-discussion + web-general
access: public
subquestion_ids: [ST3]
credibility: 3
recency: 4
bias: 2
total: 9
used: Y
---

## Топ-5 болей (из форумов, Reddit, HN)

1. **Долгий feedback loop** — commit → push → wait → check → fix → repeat
   - "Мусорные" коммиты только для тестирования
   - act решает: секунды вместо минут

2. **Нет интерактивного shell при падении**
   - act `--bash` = не то же что debug shell в нужный момент
   - GitHub Actions: нет встроенного SSH-rerun (в отличие от CircleCI)
   - Добавляют echo-statements вручную вместо debugger

3. **Непредсказуемость локального окружения (Docker image mismatch)**
   - act использует не те образы — поведение отличается от GitHub
   - "Работает локально, падает на GitHub" и наоборот

4. **Permissions и secrets**
   - GITHUB_TOKEN права другие локально
   - YAML case-sensitive (shell: vs Shell:)
   - Сложно локально тестировать secrets

5. **Нет step-level debugging**
   - Нет "пауза после шага N" как в нормальном debugger
   - ACTIONS_STEP_DEBUG даёт логи, не интерактивность

## Основные workarounds

1. **act + echo debugging** — `act -j test`, потом добавляют echo-statements, убирают перед коммитом
2. **action-tmate** — SSH в реальный GitHub runner на падении; "last resort"
3. **Множество тестовых коммитов** — draft PR, squash потом

## action-tmate: что не нравится (конкретно)
- Account suspension risk (tmate.io TOS)
- Windows compatibility ломается (Error 127, OpenSSL версии)
- SSH-строка на экране скроллится, невозможно скопировать
- "anybody can connect" без конфигурации SSH keys
- >45 минут wait иногда
- На self-hosted agents может не работать

## Что actdbg закрывает vs что остаётся
- ✅ Feedback loop (локальный запуск)
- ✅ Interactive shell с env reconstruction — это главная боль, actdbg решает лучше всех
- ⚠️ Docker mismatch — честно признаётся через `check`, но не устраняется
- ❌ Permissions/secrets — нет реального GITHUB_TOKEN
- ❌ Параллельная отладка нескольких jobs
- ❌ SSH-rerun как CircleCI (actdbg = local tool, не облачный)
