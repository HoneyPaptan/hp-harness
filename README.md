# hp

[![ci](https://github.com/HoneyPaptan/hp-harness/actions/workflows/ci.yml/badge.svg)](https://github.com/HoneyPaptan/hp-harness/actions/workflows/ci.yml)
[![release](https://img.shields.io/github/v/release/HoneyPaptan/hp-harness?sort=semver)](https://github.com/HoneyPaptan/hp-harness/releases)
[![license](https://img.shields.io/badge/license-Apache%202.0-blue)](LICENSE)

**A small, fast coding agent for the terminal, built so a run never dies.**

Most coding agents stop when you hit a usage limit, close the laptop or lose SSH. hp writes every step to a journal before it takes the next one, waits out rate limits on its own, keeps running in the background, and picks up after a crash or a reboot from the last finished step.

One static Go binary, about 8.6 MB, with no runtime to install. It starts in under 5 ms.

> hp is early (v0.0.x). Everything listed below works, but flags and the journal format can still change between releases.

## What works today

| Area | Status |
| --- | --- |
| Interactive terminal session (bare `hp`) with model picker, `/` commands and `@file` attachments | works |
| One-shot runs (`hp run "..."`), resume, list runs | works |
| Append-only journal per step, resume after Ctrl-C, crash or kill | works |
| Rate limit handling: reads the reset time, waits, retries the same step | works |
| Fallback models when a limit wait is too long (`--fallback`) | works |
| Background runs (`-d`), `hp attach`, `hp stop` | works |
| Restart dead runs after a reboot (`hp recover`, `hp autostart` on systemd and launchd) | works |
| 13 provider presets, plus any OpenAI-compatible server | works |
| Native Anthropic Messages API with prompt caching | works |
| Built-in tools: `read`, `write`, `edit`, `bash`, `grep`, `web_fetch`, `web_search` | works |
| Permission modes: `auto` (model-reviewed), `ask`, `full-access`, plus rules | works |
| `AGENTS.md` project instructions, including nested folders | works |
| Automatic context compaction and `/compact` | works |
| MCP servers (stdio and HTTP) and skills via `hp add` | works |
| Browser UI (`hp web`) that shares the same journal | works |
| Stored API keys (`hp key`) | works |

## What does not work yet

- **No sandbox.** Shell commands run as you, in the folder you started from. The `auto` mode filter is a heuristic over the command text, not isolation. Use `ask` mode or rules when that matters.
- **No `hp doctor`** health check and no `hp upgrade`. Both are planned.
- **No subagents** and no `--json` output for one-shot runs.
- **No Windows build.** Releases cover linux and macOS on amd64 and arm64.
- **MCP:** no OAuth, no resources or prompts, and servers that only speak protocol `2026-07-28` or newer are refused.
- **Reasoning is not kept** between turns. Because of that, `deepseek` runs with thinking turned off.
- **Groq quirk:** when a model on Groq calls a tool that does not exist, Groq answers HTTP 400 and the run stops. `hp resume` continues it. An automatic retry is planned.
- **`hp web` has no TLS.** It binds to `127.0.0.1` by default; on any other address the token is only as private as that network.
- **macOS autostart** does not read `~/.config/hp/env`; give it keys through `launchctl setenv` or run `hp recover` yourself.
- Reopening a finished run in `hp web` does not show the stats lines of earlier turns.

## Install

Prebuilt binaries for linux and macOS, amd64 and arm64:

```sh
curl -fsSL https://raw.githubusercontent.com/HoneyPaptan/hp-harness/main/install.sh | sh
```

The script downloads the latest release, checks its sha256 against `checksums.txt` and puts `hp` in `~/.local/bin` (no sudo). Pin a version with `HP_VERSION=v0.0.7`. Make sure `~/.local/bin` is on your `PATH`, then:

```sh
hp --version
```

### Build from source

Needs Go 1.27.3 or newer and `make`.

```sh
git clone https://github.com/HoneyPaptan/hp-harness.git
cd hp-harness
make build      # writes ./hp
make test       # unit tests, no network, fake provider
make check      # gofmt, vet, no-comments rule, core boundary, size budget
make bench      # startup and overhead bounds
```

Release builds leave HTTP/2 out of `net/http` to keep the binary small; provider traffic uses HTTP/1.1.

## Quick start

1. Store a key once. It goes to `~/.config/hp/env` with mode 0600 and is read by every hp command:

   ```sh
   hp key set GROQ_API_KEY
   ```

   An exported environment variable works too, and wins over the file.

2. Open a session in your project:

   ```sh
   cd my-project
   hp -m groq/openai/gpt-oss-120b
   ```

   Run bare `hp` with no model and the picker opens, listing the providers you have keys for.

3. Or fire a single task:

   ```sh
   hp run -m groq/openai/gpt-oss-120b "find the TODOs in this repo and fix the easy ones"
   hp runs                 # list runs
   hp resume <run-id>      # continue one
   hp -c                   # continue the latest run in this folder
   ```

Set `HP_MODEL=groq/openai/gpt-oss-120b` to skip `-m`. No key at all? Run a local model with [Ollama](https://ollama.com) and use `-m ollama/<model>`.

## Providers

`-m` takes `preset/model`. A preset with no model name is refused.

| Preset | Endpoint | Key |
| --- | --- | --- |
| `opencode` | `https://opencode.ai/inference/openai/v1` | `OPENCODE_API_KEY` |
| `openai` | `https://api.openai.com/v1` | `OPENAI_API_KEY` |
| `anthropic` | `https://api.anthropic.com/v1` | `ANTHROPIC_API_KEY` |
| `gemini` | `https://generativelanguage.googleapis.com/v1beta/openai` | `GEMINI_API_KEY` |
| `xai` | `https://api.x.ai/v1` | `XAI_API_KEY` |
| `deepseek` | `https://api.deepseek.com` | `DEEPSEEK_API_KEY` |
| `mistral` | `https://api.mistral.ai/v1` | `MISTRAL_API_KEY` |
| `cerebras` | `https://api.cerebras.ai/v1` | `CEREBRAS_API_KEY` |
| `together` | `https://api.together.ai/v1` | `TOGETHER_API_KEY` |
| `fireworks` | `https://api.fireworks.ai/inference/v1` | `FIREWORKS_API_KEY` |
| `openrouter` | `https://openrouter.ai/api/v1` | `OPENROUTER_API_KEY` |
| `groq` | `https://api.groq.com/openai/v1` | `GROQ_API_KEY` |
| `ollama` | `http://127.0.0.1:11434/v1` | none |

`hp providers` shows which keys are set without printing them. `anthropic` speaks the Messages API with prompt caching on the system prompt, tools and history, and uses `max_tokens` 16000 unless you pass `--max-tokens`. Everything else speaks OpenAI chat completions. Any other compatible server works with `--base-url` and `--key-env`.

## A run that never dies

### Journal

Every step is appended to a journal in `~/.local/state/hp/runs` before the next one starts, so a crash costs at most one step. Ctrl-C at any point leaves the run resumable.

### Rate limits and fallbacks

```sh
hp run -m groq/openai/gpt-oss-120b --fallback groq/openai/gpt-oss-20b "refactor the parser"
```

On a 429, 503 or 529, hp reads the reset time from `Retry-After`, `x-ratelimit-reset-*` or `anthropic-ratelimit-*-reset`, prints when it will continue, waits and retries the same step. If the wait is longer than `--switch-after` (default 20s) and a `--fallback` model is free, it switches instead. `--max-wait` (default 6h) caps a single wait; past it hp exits with code 4 and the run stays resumable. Waits and switches are journaled, so a resume stays on the model the run was using. Truncated provider streams are retried too.

### Background runs

```sh
hp run -d "refactor the parser"   # returns at once
hp attach <run-id>                # follow it; Ctrl-C detaches, the run keeps going
hp stop <run-id>
```

Output goes to `<run-id>.log` next to the journal. `hp runs` marks live runs as `running`, and a second resume of a live run is refused.

### Reboot survival

```sh
hp recover             # resume every detached run whose process is gone
hp recover --dry-run   # only list them
hp autostart on        # run recover at login (or at boot with linger)
```

`hp recover` skips runs that were stopped on purpose, runs older than `--max-age` (default 7d), runs whose API key is missing, and runs that crashed three recoveries in a row without progress, so a crash loop cannot burn credit. A lock file stops two recovers from starting the same run twice.

On linux, `hp autostart on` writes and enables `~/.config/systemd/user/hp-recover.service`, which reads keys from `~/.config/hp/env`. Add `loginctl enable-linger $USER` to recover at boot without logging in. On macOS it writes a launchd agent with `RunAtLoad`.

## Session

Bare `hp` on a terminal opens an inline session that prints into normal scrollback. Every follow-up continues the same journaled run. The footer shows the permission mode, model and Jev state, for example `auto · groq/openai/gpt-oss-120b · jev off`.

| Key or command | Does |
| --- | --- |
| `/model` | pick a provider with a key, then a model from its `/models` list |
| `/model preset/name` | switch directly |
| `/permissions [ask\|auto\|full-access\|reset]` | show or set the mode; Shift+Tab cycles it |
| `/compact` | compact the context now |
| `/key NAME` | store an API key with masked input |
| `/jev on\|off` | toggle Jev |
| `/new`, `/runs`, `/help`, `/quit` | |
| `@path` | attach a file (64 KB per file, 256 KB per message) |
| Ctrl-C or Esc | stop the turn; on an empty prompt, Ctrl-C twice or Ctrl-D quits |

Slash commands stay local and send nothing to the model. `//text` sends a literal `/text`. The session opens its provider connection at start (and again after 60s idle), so the first turn skips the TCP and TLS handshake. Time to first token shows in the status line. `NO_COLOR` or `TERM=dumb` turns colour off.

## Tools and permissions

Built-in tools: `read`, `write`, `edit`, `bash`, `grep`, `web_fetch`, `web_search` (only when `EXA_API_KEY` is set), `skill`, and `search` with `--jev`.

Every tool call goes through a permission gate. Three modes:

- **`auto`** (default): reads, searches and known simple commands such as `git status`, `go test ./...` or `npm run build` run straight away. Edits inside the working folder are allowed unless the path is sensitive (`.env`, `.git`, `~/.ssh`, `~/.config/hp`, shell rc files and similar). Anything else is reviewed by the session's own model, which must either clear the action or hold it with a quoted reason; any failure holds.
- **`ask`**: every write, edit, command and MCP call waits for `y` (yes), `a` (always for this), or `n`. Non-interactive runs hold instead of asking.
- **`full-access`**: no checks. `hp run --full-access` does this for one run.

Rules in `~/.config/hp/settings.json` run before everything else; the last matching rule wins:

```json
{
  "permission_mode": "auto",
  "permission": {
    "*": "ask",
    "bash": { "git *": "allow", "git push *": "deny" },
    "web_fetch": { "https://pkg.go.dev/*": "allow" }
  }
}
```

`HP_PERMISSION_MODE` sets the starting mode. `hp permissions --json` prints the resolved setup.

The `bash` tool runs `bash -c` with your login shell's environment (captured once, 3s limit), so your PATH from fish, zsh or bash is there. Every command gets `AI_AGENT=hp`. `HP_LOGIN_ENV=0` uses hp's own environment instead.

`web_fetch` reads public `http` and `https` pages as plain text (64 KB cap) and refuses loopback, private, link local and tailnet addresses at connect time, so redirects cannot get around it. `HP_WEB_ALLOW_PRIVATE=1` lifts that. `web_search` uses [Exa](https://exa.ai). `"web": false` in settings removes both.

## Project instructions

hp loads `AGENTS.md` into the system prompt: `~/.config/hp/AGENTS.md` first, then each parent folder below your home, then the working folder. Closer files win. A nested one like `apps/web/AGENTS.md` is attached the first time the agent touches a file under that folder. Each file is cut at 32 KB, all of them at 64 KB. `"context": false` in settings turns this off.

## Compaction

When the last request used 80% of the usable context window, hp swaps older history for a handoff: your earlier prompts word for word plus a summary written by the same model. The newest ~5% of the window stays as is, and a tool call is never split from its result. On a context overflow error it compacts and retries once. The full history stays in the journal.

```json
{ "auto_compact_percent": 70, "context_windows": { "groq/openai/gpt-oss-120b": 131072 } }
```

`auto_compact_percent` accepts 10 to 80. Unknown models are assumed to have 128000 tokens.

## MCP servers and skills

```sh
hp add fs -- npx -y @modelcontextprotocol/server-filesystem ~/notes
hp add docs https://example.com/mcp --header 'Authorization=Bearer $DOCS_TOKEN'
hp add gh --env 'GITHUB_TOKEN=$GITHUB_TOKEN' --timeout 5m -- github-mcp-server stdio
hp add ./my-skill
hp add https://github.com/someone/some-skill.git
hp list
hp add --refresh fs
hp remove fs
```

`hp add` connects once, caches the tool list and prints `added fs: 12 tools`; nothing is saved if it cannot connect. Quote `$NAME` so the file keeps the reference and hp reads the value when it connects.

- Connections live only in `~/.config/hp/connections.json` (mode 0600), never in a project, so a cloned repo cannot start a command.
- Servers start on first use, not at launch, and stop when the run ends. A crashed server restarts once; a stdio server that times out three times in a row is given up for that run.
- Stdio servers get a minimal environment (`PATH`, `HOME`, `USER`, `SHELL`, `TERM`, locale, `TMPDIR` and your `--env`), so your model keys stay with hp.
- Server tools appear as `<server>__<tool>`. With more than 8 tools, they stay out of the request and the model searches them through a `tools` tool instead.

Skills are folders with a `SKILL.md`, found in `.hp/skills`, `.claude/skills` or `~/.config/hp/skills`. The model sees their names and loads one with the `skill` tool when it needs it. Installing from git is a shallow clone and none of its code runs.

## Web UI

```sh
hp web -m groq/openai/gpt-oss-120b
```

Serves a page from the binary and prints a URL with a one-time token. It looks and works like the terminal session and uses the same journal: close the tab and the run keeps going, reopen and it replays and rejoins. Binds `127.0.0.1:7878` by default; `--listen host:port` picks another address (use a concrete host, not `0.0.0.0`). The server checks the token, `Host` and `Origin`. Anyone with the URL can run shell commands through the agent, so treat it like a password.

## Jev (optional)

```sh
hp run --jev -m groq/openai/gpt-oss-120b "where are rate limit headers parsed?"
```

`--jev` or `HP_JEV=1` turns on Jev, TypeSafe's judgment model, for one run. It adds a `search` tool that finds code by meaning, and picks which tools to send when there are more than 8. It needs `TYPESAFE_API_KEY` or `OPENROUTER_API_KEY`. File paths and excerpts are sent to the Jev service for search (secret-looking files like `.env` and `*.pem` are withheld); for tool choice only the first prompt, last reply and tool names. Off by default.

## Speed

`make bench` measures hp against a local fake server that replies instantly, so the numbers are hp's own overhead. CI enforces the bounds.

| Measure | Laptop | CI bound |
| --- | --- | --- |
| `hp --version` | 3.8 ms | 10 ms |
| `hp runs` with 500 journals | 6.9 ms | 100 ms |
| `hp run` start to first byte | 5.0 ms | 15 ms |
| Peak RSS | 15.8 MB | 40 MB |

The stripped binary must stay under the budget in `.size-budget` (currently 8.6 MB of 9.0 MB).

## Design

- **The core never imports `os`, `os/exec`, `net`, `net/http` or `syscall`.** It talks to the world through one `Host` interface, so it can run in a terminal, a daemon or another app. `make check` enforces this.
- **Standard library first.** Each new dependency has to justify its size.
- **Tests never touch the network** and never call a real model; they use a fake provider.

```
cmd/hp            entry point
internal/core     agent loop, compaction
internal/journal  append-only step log, replay, resume
internal/provider OpenAI-compatible and Anthropic adapters, retries
internal/tool     built-in tools
internal/permit   permission modes, rules, review
internal/mcp      MCP client
internal/cli      commands, session, detach, recover
internal/tui      terminal UI
internal/web      browser UI
```

## Files

| Path | Holds |
| --- | --- |
| `~/.local/state/hp/runs/` | journals and background run logs |
| `~/.config/hp/env` | stored API keys (0600) |
| `~/.config/hp/settings.json` | permissions, compaction, context, web settings |
| `~/.config/hp/connections.json` | MCP servers (0600) |
| `~/.config/hp/skills/` | installed skills |
| `~/.config/hp/AGENTS.md` | your personal instructions |

## License

[Apache 2.0](LICENSE)
