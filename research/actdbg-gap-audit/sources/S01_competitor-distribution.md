---
id: S01
slug: competitor-distribution
title: Дистрибуция и discoverability конкурентов (lazygit, act, dive, k9s)
channel: code-github + web-general
access: public
subquestion_ids: [ST1]
credibility: 4
recency: 5
bias: 2
total: 11
used: Y
---

## Ключевые факты

### Методы установки по инструментам:
- **lazygit** (79.4k ★): Homebrew, apt, Scoop, go install, Arch, Fedora, NixOS, Conda, Chocolatey, Winget — 10+ методов
- **act** (70.8k ★): manual build (go install); Homebrew есть через https://github.com/nektos/act#installation
- **dive** (54.3k ★): Homebrew, MacPorts, Choco, Scoop, Pacman, .deb/.rpm, go install, Docker — 8+ методов
- **k9s** (34k ★): Homebrew, MacPorts, snap, pacman, zypper, pkg, dnf, Winget, Scoop, Choco, go install — 11+ методов
- **actdbg** (~200-300 ★): go install only

### Что есть у конкурентов, нет у actdbg:
| Gap | Критичность (1-5) |
|-----|-------------------|
| Homebrew tap | 5/5 — macOS-пользователи ожидают brew install |
| Binary releases (GitHub Releases) | 5/5 — Linux без Go не может установить |
| GitHub Topics | 3/5 — SEO/discoverability в GitHub Search |
| CHANGELOG / Releases page | 3/5 — сигнал "живой проект" |
| CONTRIBUTING.md | 2/5 — для потенциальных контрибьюторов |
| Multiple install methods | 4/5 — снижает трение у ~70% потенциальных юзеров |

### UX journey без Homebrew:
1. Discovery: нет Topics → не найдёт в GitHub Search
2. Install: только go install → нужен Go компилятор (у 30-40% macOS-разработчиков нет)
3. Validation: нет CHANGELOG → неясно что менялось, выглядит неживым

### Best practices (GoReleaser):
- GoReleaser + GitHub Actions = стандарт для Go dev-tools
- Автоматически: бинарники Linux/macOS amd64+arm64, .deb, .rpm, Homebrew formula
- Настройка: 2-4 часа однократно, потом автоматически на каждом теге
