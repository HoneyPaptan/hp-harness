# hp

A small, fast coding agent for the terminal, built so a run never dies.

> Early work in progress. Runs, journals, resumes, waits out rate limits, falls back to other models, runs in the background with `-d`, and restarts detached runs after a crash, a kill or a reboot with `hp recover`. Bare `hp` opens an inline session, and `hp add` connects MCP servers and skills.

## Why

Coding agents stop when you hit a usage limit, close the laptop or lose SSH. hp keeps going.

- **Survives limits.** When a provider says you are out of quota, hp reads the reset time, waits, and continues from the same step. Or it falls back to another model you allowed.
- **Survives you leaving.** Start with `-d` and close the terminal or lose SSH; the run carries on in the background. `hp attach` puts you back in.
- **Loses nothing.** Every step is appended to a journal before the next one starts, so a crash costs at most one step.

## Principles

- **Fast first token.** Connections open while you type, the system prompt stays short, tool schemas load only when needed. The status bar shows real time to first token.
- **One command connections.** `hp add` wires up an MCP server or a skill from a name, a folder or a URL.
- **Small.** One static binary with a hard size budget enforced in CI.
- **Anywhere.** The core talks to the world through one `Host` interface, so it can run in a terminal, a daemon or inside another app.

## Build

```
make build
```

Requires Go 1.27 or newer. Release builds omit HTTP/2 from `net/http` to keep the binary small; provider traffic uses HTTP/1.1.

## Install

Static binaries for linux and macOS, amd64 and arm64, are attached to every release with a `checksums.txt`:

```
curl -fsSL https://raw.githubusercontent.com/HoneyPaptan/hp-harness/main/install.sh | sh
```

or run the script straight from a checkout. It downloads the latest release, verifies the sha256 checksum and puts the binary in `~/.local/bin` without sudo. Set `HP_VERSION=v0.1.0` to pin a release. The checksum guards against a broken download, not a tampered release. Every tagged build runs tests and cross builds first, and the version matches the tag:

```
hp --version
```

## Use

```
export OPENCODE_API_KEY=...
./hp -m opencode/grok-4.6
./hp run -m opencode/grok-4.6 "find the TODOs in this repo and fix the easy ones"
./hp runs
./hp resume <run-id>
./hp recover
```

`-m` takes `preset/model`, for example `hp -m groq/openai/gpt-oss-120b`. A preset with no model name (`groq` or `groq/`) is refused. Presets:

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

`hp providers` lists these with whether each key is set, and never prints a key. `anthropic` speaks the Messages API, with prompt caching on the system prompt, the tools and the conversation so far; the rest speak OpenAI chat completions. `deepseek` runs with thinking off, because its API requires reasoning to be sent back on every tool call and hp does not keep reasoning. `anthropic` uses 16000 for `max_tokens` unless you pass `--max-tokens`.

Any other OpenAI compatible server works with `--base-url` and `--key-env`. `HP_MODEL` sets a default model.

Text streams to stdout, tool calls print one line each to stderr, and a summary line ends the run. Press Ctrl-C at any time: the run stays resumable. Journals live in `~/.local/state/hp/runs`.

The model can paste files, edit code and run shell commands through the built-in tools. hp has no permission prompts yet: it runs the commands the model asks for, in the directory the run started from. Keep that in mind before pointing it at a prompt.

Built-in tools: `read`, `write`, `edit`, `bash`, `grep`, plus `search` with `--jev`. Skills are folders with a `SKILL.md` in `.hp/skills`, `.claude/skills` or `~/.config/hp/skills`; the model sees their names and loads one with the `skill` tool when it needs it. MCP servers are added with `hp add`.

## Keys

`hp key set OPENAI_API_KEY` reads the key without echo when stdin is a terminal, or one line from stdin otherwise, and writes `NAME=value` to `~/.config/hp/env` with mode 0600. Every hp command reads that file at start, so a key is stored once. A key that is already set in the real environment wins over the file.

`hp key list` prints each known name with its source, `file` or `env`, and never prints a value. `hp key rm NAME` removes one line.

The file uses the same syntax as a systemd `EnvironmentFile`, so `hp autostart` recovery reads the same keys. Names use upper case letters, digits and underscores.

## Session

Bare `hp` on a terminal opens a session. Tool shells inherit your login shell environment once at startup; set `HP_LOGIN_ENV=0` to use hp's process environment instead. A header line shows the version, the conversation prints inline into normal scrollback with one dim status line after each turn, and a footer under the prompt shows the permission mode, the model and whether Jev is on, for example `auto · groq/openai/gpt-oss-120b · jev off`. Every follow-up continues the same journaled run, so a session survives like any other run.

With no `-m` and no `HP_MODEL`, the session opens with the footer `auto · no model · /model` and the model picker already up. A prompt sent before a model is picked stays in the editor until one is.

```
./hp -m opencode/grok-4.6
./hp -c
```

`-c` continues the latest run in this directory, and finishes it first if it stopped midway. `-m`, `-C`, `--jev`, `--base-url` and `--key-env` work here too. Without a terminal, bare `hp` prints usage. A run in a live session shows as `running`, and a second session or `hp resume` on it is refused.

Ctrl-C or Esc stops a turn and returns to the prompt; keys typed during a turn wait there as the next draft. At the prompt, Ctrl-C clears the text, twice on an empty prompt quits, and so does Ctrl-D.

Commands stay local and send nothing to the model: `/help`; `/model` opens a picker of providers that have a key, then lists that provider's models from its `/models` endpoint (type to filter, arrows and Enter to pick; if the list cannot be fetched, type the name); `/model groq` opens the picker at groq's list; `/model preset/name` switches directly; `/jev on` and `/jev off` switch Jev for following turns and are kept on resume; `/permissions` shows or sets the permission mode; `/key NAME` stores an API key with masked input, the same way as `hp key set`; `/new` starts a fresh run; `/runs`; `/quit`. `//text` sends a literal `/text`.

Typing `/` lists the commands, filtered as you type; Tab or Enter completes. Typing `@` lists files under the working folder (hidden folders, `node_modules`, `vendor`, links and simple top level `.gitignore` patterns skipped, at most 200). Each `@path` in a sent prompt attaches that file to the message: up to 64 KB per file and 256 KB per message, binary files skipped, and paths outside the working folder or through a symlink left as plain text. The transcript shows what you typed; the journal keeps what the model saw.

A session opens its connection to the provider at start, and again on the first key after 60 seconds idle, so the first turn skips the TCP and TLS handshake. `hp web` does the same at page load. The probe is an unauthenticated `HEAD` on the base URL; if it fails, nothing shows and the turn connects as usual. On models that stream reasoning the activity line reads `thinking…`, time to first token stops there, and reasoning is neither printed nor journaled.

`NO_COLOR` or `TERM=dumb` turns colour off.

## Project instructions

hp reads `AGENTS.md` into the system prompt, like fx: your personal `~/.config/hp/AGENTS.md` first, then the file in each parent folder below your home folder, then the one in the working folder. Closer files come later and win on conflicts; your own messages win over all of them. Each file is cut at 32 KB and all of them together at 64 KB.

A nested `AGENTS.md`, such as `apps/web/AGENTS.md`, loads the first time the agent reads, edits or writes a file under that folder, attached to that tool result once per run. A resumed run keeps the instructions it started with.

Set `"context": false` in `~/.config/hp/settings.json` to turn all of this off.

The bash tool runs commands with `bash -c`, using the environment of your login shell (`$SHELL -l -c 'env -0'`, read once per process, 3 second limit), so PATH entries and tools from your fish, zsh or bash config are there. Every command also gets `AI_AGENT=hp`. `HP_LOGIN_ENV=0` keeps hp's own environment instead.

## Permissions

Every tool call passes through a permission gate before it runs. The footer shows the active mode: `auto`, `ask`, or `full-access`. `auto` is the default when nothing else is set.

In `auto`, reads, searches and similar lookups run without asking. Edits and writes inside the working folder are allowed when the path is not sensitive (for example `.env`, anything under `.git`, `~/.ssh`, `~/.config/hp`, `~/.gnupg`, or your shell rc files). Simple, known shell commands run without asking, including `git status`, `git diff`, `go test ./...`, `make test`, and `npm run build`. Chained, piped or redirected commands, quoted flags, globs, paths outside the working folder, and writes through symlinks are not treated as routine.

Anything else in `auto` goes to review. The session's model reads the exact action plus recent user messages and tool output, and must reply with JSON that either clears the action or holds it with a quoted proof from that context. The check runs twice when the first reply holds; any parse error or failed check holds the action. The agent sees `held: <reason>`.

In `ask`, every write, edit, command and MCP call needs your approval at the prompt (reads and searches still run): Yes, Yes and don't ask again for this, or No. Keys are `y`, `a`, `n`, and Esc for No. `hp run`, detached runs and the web UI cannot answer; those runs hold with an ask verdict instead.

Rules live in `~/.config/hp/settings.json`. Example:

```
{"permission_mode": "auto", "permission": {"*": "ask", "bash": {"git *": "allow", "git push *": "deny"}}}
```

Patterns use `*` wildcards. The last matching rule wins. Rules are checked before session grants and before routine or review.

```
/permissions
/permissions ask|auto|full-access|reset
```

Shift+Tab cycles `auto`, `ask`, and `full-access`. `/permissions reset` clears session grants and sets `ask`. `HP_PERMISSION_MODE` sets the starting mode for a new run. `hp run --full-access` skips all checks for that run. `hp permissions --json` prints the resolved mode, its source, the settings path, and parsed rules. A mode chosen in a session is journaled and kept on resume.

Routine command matching looks at the command text before hp runs it through `bash -c`, so it is a filter, not a sandbox. Use `ask` or rules when you need tighter control.

## Web

```
./hp web -m groq/openai/gpt-oss-120b
```

`hp web` serves a page from the binary and prints a URL with a one-time token to stderr. The page looks like the terminal session: the same footer, and `/model preset/name`, `/new`, `/runs` and `/help` typed into the prompt. A run started there uses the same journal as `hp run`: closing the tab leaves it going, reopening replays the journal and rejoins live, and stopping the server leaves it resumable. Runs live in another process are shown but refused.

`-m`, `--jev`, `-C` and the other `hp run` flags work here. `--port` picks the port (default 7878, or a free one with a note if it is taken). It binds 127.0.0.1 unless `--listen host:port` says otherwise. The token is sent without TLS, so a non-loopback `--listen` is only as private as that network. The agent can run shell commands: anyone with the URL can too. The server checks the token, `Host` and `Origin` on requests, and only `127.0.0.1`, `localhost` and the `--listen` host are accepted, so use a concrete host rather than `0.0.0.0`.

## Jev

```
export TYPESAFE_API_KEY=...
./hp run --jev -m groq/openai/gpt-oss-120b "where are rate limit headers parsed?"
```

`--jev` or `HP_JEV=1` turns on Jev, TypeSafe's yes/no judgment model, for one run. It is off by default, and when off nothing below happens. It needs `TYPESAFE_API_KEY` or `OPENROUTER_API_KEY`; if both are set, TypeSafe wins. With neither set, hp exits with code 2 before it starts. `HP_JEV_MODEL` picks the model.

With Jev on, the `search` tool finds code by meaning and lists `path:start-end` ranges with scores, best first. When a turn would send more than 8 tools, Jev also picks the ones the next step is likely to need; if that fails, hp sends them all. The summary line adds Jev requests and tokens.

What leaves the machine: for `search`, file paths and file excerpts go to the Jev service, and secret-looking files such as `.env`, `*.pem` and `id_rsa` are withheld. For tool choice, the first user message, the last assistant text and tool names go there, never tool output.

The setting is journaled, so a run stays resumable with Jev on: `hp resume` reads it from the journal and ignores `HP_JEV`, and the key must still be set.

## Connections

```
./hp add fs -- npx -y @modelcontextprotocol/server-filesystem /home/me/notes
./hp add docs https://example.com/mcp --header 'Authorization=Bearer $DOCS_TOKEN'
./hp add gh --env 'GITHUB_TOKEN=$GITHUB_TOKEN' --timeout 5m -- github-mcp-server stdio
./hp add ./my-skill
./hp add https://github.com/someone/some-skill.git
./hp list
./hp add --refresh fs
./hp remove fs
```

`hp add` connects to a server once, reads its tools, stops it and prints `added fs: 12 tools`. If it cannot connect, nothing is saved and the error shows the end of the server's stderr. Run `--refresh` when a server's tools change. A folder or git URL with a `SKILL.md` is copied to `~/.config/hp/skills/<name>`; a git repo is cloned shallow, may hold the skill at its root or one folder down, and none of its code runs. Symlinks in a skill are not copied.

Connections live in `~/.config/hp/connections.json` with the tool schemas cached next to it, both mode 0600. They are only ever read from there, never from a project, so a cloned repo cannot start a command. Quote `$NAME` so your shell leaves it alone: the file keeps the reference, and hp reads the value from its environment when it connects. A literal value is stored as typed.

A stdio server runs in the folder you added it from. It gets `PATH`, `HOME`, `USER`, `LOGNAME`, `SHELL`, `TERM`, `LANG`, `LC_ALL`, `TMPDIR` and your `--env` values, and not the rest of your environment, so model API keys stay with hp. HTTP servers need `https`, or `http` on a loopback address, and redirects are not followed.

Connections add nothing to start time. A server starts when one of its tools is first called, and stops when the run ends; in a session it stops when you quit or use `/new`, and in `hp web` it stays up between a run's turns until the run has been idle for 10 minutes or the server shuts down. A tool that fails, or a server that will not start, gives the model an error and the run carries on. A server that crashes is restarted once on the next call.

Calls time out after 60 seconds, or the `--timeout` you gave `hp add`. A stdio server that times out is killed with its process group and restarted on the next call; after 3 timeouts in a row with no successful call, hp stops restarting it for that run. An HTTP server stays connected.

A server's tools appear as `<server>__<tool>`. When a run would have more than 8 tools, its server tools stay out of the request and the model gets a `tools` tool to search them by name or description; up to 5 matches join the request from the next turn, and stay loaded after `hp resume`. With `--jev` every tool goes to the chooser instead. If you remove a connection, runs that used it still resume; a new call to its tools returns an error.

hp speaks the handshake versions of MCP, up to `2025-11-25`. Servers that only speak `2026-07-28` or later are refused with the server's error. OAuth, MCP resources and prompts are not supported.

## Speed

`make bench` builds hp and measures it against a local fake server that replies at once, so the numbers are hp's own cost and exclude any provider. The same check runs in CI, where a slower shared runner gets the bounds below. Medians from one laptop run:

| Measure | Local | CI bound |
| --- | --- | --- |
| `hp --version` | 4.6 ms | 10 ms |
| `hp runs` with 500 journals | 6.5 ms | 100 ms |
| `hp run` start to first byte | 4.4 ms | 15 ms |
| Peak RSS | 15.5 MB | 40 MB |

`-d` on `run` or `resume` starts the run in its own session and returns. Output goes to `<run-id>.log` next to the journal. `hp attach` prints it and follows along; Ctrl-C detaches and leaves the run going. `hp runs` shows live runs as `running`, and a second `hp resume` on a live run is refused. A run ended by `hp stop` or interrupted is marked in its journal, so recovery knows the difference between that and a crash.

## Limits and background runs

```
./hp run -d -m groq/openai/gpt-oss-120b --fallback groq/openai/gpt-oss-20b "refactor the parser"
./hp attach <run-id>
./hp stop <run-id>
```

On a rate limit (429) or overload (503, 529) hp reads the reset time from `Retry-After` or the `x-ratelimit-reset-*` and `anthropic-ratelimit-*-reset` headers, prints when it will continue, waits and retries the same step. If the wait is longer than `--switch-after` (default 20s) and a `--fallback` model is free, it switches at once instead. `--max-wait` (default 6h) caps one wait; past it hp stops with exit code 4 and the run stays resumable. Waits and switches are journaled, so `hp resume` keeps the model the run was on.

`-d` on `run` or `resume` starts the run in its own session and returns. Output goes to `<run-id>.log` next to the journal. `hp attach` prints it and follows along; Ctrl-C detaches and leaves the run going. `hp runs` shows live runs as `running`, and a second `hp resume` on a live run is refused.

## Reboot survival

```
./hp run -d "refactor the parser"
./hp recover            # resumes every detached run whose process is gone
./hp recover --dry-run  # just list them
```

A run survives usage limits, closed terminals, lost SSH and reboots: `hp recover` scans the runs, and for each detached, unfinished run that was not stopped on purpose and has no live process, it starts a detached resume from the last completed step and prints `recovered <run-id>`. Keep `hp recover` in a login script or autostart it, and the run picks up where it died. Runs older than `--max-age` (default 7d) are skipped, and a run that crashed three recoveries in a row without writing a new journal record is left alone with a note, so a crash loop cannot burn credit. A run whose API key is not set is skipped with a note too; the others carry on. Locked with a lock file, so two `hp recover` at once start nothing twice.

```
./hp autostart on       # or off, or status
```

On linux this writes `~/.config/systemd/user/hp-recover.service`, a oneshot unit enabled with `systemctl --user enable`; combined with `loginctl enable-linger $USER` it also recovers runs at boot without login. On macOS it writes a launchd agent in `~/Library/LaunchAgents` with `RunAtLoad`.

The unit holds no API keys. Recovery reads them from `~/.config/hp/env`, mode 0600, the same file `hp key set` writes:

```
XAI_API_KEY=...
GROQ_API_KEY=...
```

The linux unit picks it up with `EnvironmentFile=-%h/.config/hp/env`. On macOS the launchd agent gets no such file: give the keys to the login environment (for example with `launchctl setenv KEY value`) or run `hp recover` yourself.

## License

Apache 2.0
