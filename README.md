<img src="docs/readme/resume.webp" alt="Terminal session: hp run is interrupted with Ctrl-C while a tool runs, hp runs lists the run as resumable, and hp resume finishes it and writes TODOS.md" width="100%">

# hp

hp is a coding agent for the terminal. Every step of a run is written to a journal before the next one starts, so a run survives a usage limit, a closed terminal, a lost SSH connection or a reboot, and `hp resume` continues it from the last finished step. The picture above is a real session: Ctrl-C while a tool was running, then `hp resume` on the same run.

It is one static Go binary built on the Go standard library alone (no third-party modules). It talks to 13 provider presets (Anthropic's Messages API, plus OpenAI-compatible endpoints such as Groq, OpenRouter, xAI and a local Ollama), runs the usual file, shell and web tools, and connects to MCP servers and skills with `hp add`.

Most agents keep the conversation in memory, so when the process dies the work is gone. hp keeps it on disk and treats a dead process as a pause. It is early (v0.0.x): flags and the journal format can still change.

## What is special

- **Journal and resume.** One append-only file per run in `~/.local/state/hp/runs`. `hp runs` lists them, `hp resume <id>` continues one (a tool that was running when the process died is marked interrupted and runs again), `hp run -d` runs in the background, `hp attach <id>` follows it and `hp stop <id>` ends it. `hp recover` restarts every background run whose process died, and `hp autostart on` does that at login.
- **Limit waits and fallback.** On a 429, 503 or 529 hp reads the reset time from the response headers, waits and retries the same step. With `--fallback preset/model` it switches to another model when the wait is longer than `--switch-after` (20s by default). `--max-wait` caps one wait (6h by default).
- **Permissions.** `auto` (default) lets reads and known simple commands through and has the session's own model review the rest, `ask` prompts for every write and command, `full-access` checks nothing. Rules in `settings.json` run first. There is no sandbox: shell commands run as you.
- **MCP and skills.** `hp add` takes a stdio command, an HTTP MCP URL, or a skill folder or git URL. Connections live only in `~/.config/hp/connections.json`, never in a project, so a cloned repo cannot start a command.
- **Size and boundary.** The stripped binary has to stay under the byte budget in `.size-budget` (9,011,200). `internal/core` and `internal/permit` may not import `os`, `os/exec`, `net`, `net/http` or `syscall`; they reach the world through one `Host` interface. `make check` enforces both.

## Install and run

```sh
curl -fsSL https://raw.githubusercontent.com/HoneyPaptan/hp-harness/main/install.sh | sh
hp key set GROQ_API_KEY
export HP_MODEL=groq/openai/gpt-oss-120b
cd my-project
hp
hp run "find the TODOs in this repo and list them in TODOS.md"
hp runs
hp resume <run-id>
hp run -d "refactor the parser"
hp attach <run-id>
```

The installer supports linux and macOS on amd64 and arm64, checks the sha256 against `checksums.txt` and puts `hp` in `~/.local/bin`. `hp key set` stores the key in `~/.config/hp/env` with mode 0600. Bare `hp` opens an interactive session (without `HP_MODEL` or `-m` it opens a model picker first). `hp providers` lists the presets and which keys are set.

To build and check from source (Go 1.27.3 or newer):

```sh
git clone https://github.com/HoneyPaptan/hp-harness.git
cd hp-harness
make build   # writes ./hp
make test    # no network, fake provider
make check   # gofmt, vet, no comments, core boundary, size budget
make bench   # startup, `hp runs`, first byte and memory bounds
```

## Size and speed

The stripped binary is 8,589,575 bytes (8.6 MB), measured with `make check` against the 9,011,200 byte budget. `make bench` runs hp against a local fake server that replies at once, so it measures hp's own overhead. Median of five runs today on a busy laptop, with the bound CI enforces:

| Measure | Median | Bound |
| --- | --- | --- |
| `hp --version` | 6.1 ms | 10 ms |
| `hp runs` with 500 journals | 10.7 ms | 100 ms |
| `hp run` start to first byte | 8.0 ms | 15 ms |
| Peak RSS | 12.1 MB | 40 MB |

## Folders

```
cmd/hp              entry point
internal/core       agent loop, compaction, limit waits
internal/journal    append-only step log, replay, resume
internal/provider   OpenAI-compatible and Anthropic adapters
internal/tool       built-in tools
internal/permit     permission modes and rules
internal/mcp        MCP client
internal/cli        commands, session, background runs, recover
internal/tui        terminal UI
internal/web        local web UI
tools/              dev tools used by checks, never shipped
```

## Credits

hp follows the shape of [fx](https://github.com/vercel-labs/fx) (Apache-2.0): its ask, auto and full-access permission modes, and its aim of keeping the binary small. Web search uses [Exa](https://exa.ai) when `EXA_API_KEY` is set. MCP servers speak the [Model Context Protocol](https://modelcontextprotocol.io). hp itself is licensed under [Apache 2.0](LICENSE).
