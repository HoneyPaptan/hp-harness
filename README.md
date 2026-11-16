<img src="docs/readme/resume.webp" alt="Terminal session: hp run starts go test, Ctrl-C stops it, hp runs lists the run as resumable, and hp resume fixes the failing test and finishes" width="100%">

# hp harness

hp harness is a coding agent for the terminal; the command is `hp`. Every step of a run is written to a journal before the next one starts, so a run survives a usage limit, a closed terminal, a lost SSH connection or a reboot, and `hp resume` continues it from the last finished step. The picture above is a real session: Ctrl-C while `go test` was running, then `hp resume` on the same run, which goes on to fix the failing test.

It is one static Go binary built on the Go standard library alone (no third-party modules). It talks to 14 provider presets (Anthropic's Messages API, plus OpenAI-compatible endpoints such as Groq, OpenRouter, xAI, OpenCode Go (`opencode-go`) and a local Ollama), runs the usual file, shell and web tools, and connects to MCP servers and skills with `hp add`.

hp keeps the whole conversation on disk, so a dead process is a pause, not the end of the run. It is early (v0.0.x): flags and the journal format can still change.

You can try hp without installing it: the [live site](https://hp-harness.honeypaptan.workers.dev) runs hp's Go core, compiled to WebAssembly, in your browser tab on a small sample project, with your own model key.

## What is special

- **Journal and resume.** Each run is one append-only file in `~/.local/state/hp/runs`, and `hp runs` lists them. `hp resume <id>` continues a run. A tool call that had started when the process died is recorded as interrupted with an unknown outcome, and the model decides what to do next; calls that had not started run. `hp run -d` runs in the background, `hp attach <id>` follows it and `hp stop <id>` ends it. `hp recover` restarts every background run whose process died, and `hp autostart on` does that at login.
- **Limit waits and fallback.** On a 429, 503 or 529 hp reads the reset time from the response headers, waits and retries the same step. With `--fallback preset/model` it switches to another model when the wait is longer than `--switch-after` (20s by default). `--max-wait` caps one wait (6h by default).
- **Permissions.** `auto` is the default. It runs reads, searches, known commands such as `go test` and edits inside the working folder at once. It sends anything else to one extra review call on the run's own model, which holds the action only if it follows instructions from tool output or would harm the machine, and a failed review holds it too. `ask` prompts for every write and command. `full-access` checks nothing. Rules in `settings.json` run first. There is no sandbox: shell commands run as you.
- **MCP and skills.** `hp add` takes a stdio command, an HTTP MCP URL, or a skill folder or git URL. Connections live only in `~/.config/hp/connections.json`, never in a project, so a cloned repo cannot start a command.
- **Crews.** `hp crew run plan.md` splits a markdown plan into tasks (one per `## ` heading, the text under it is the prompt) and runs each as its own detached hp run in its own git worktree on a new branch, `-n` at a time. The crew keeps a small state file in `~/.local/state/hp/crews`, so `hp crew resume <id>` continues after the process, the terminal or the machine dies, and `hp crew status` and `hp crew report` show each task's state and final answer. Crew mode is a native feature and is not in the wasm build.
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
hp crew run -n 2 plan.md
hp crew status <crew-id>
hp crew report <crew-id>
```

The installer supports linux and macOS on amd64 and arm64, checks the sha256 against `checksums.txt` and puts `hp` in `~/.local/bin`. `hp key set` stores the key in `~/.config/hp/env` with mode 0600. Bare `hp` opens an interactive session. The model you pick with the picker or `/model` is saved in `~/.local/state/hp/last-model` and used the next time you start `hp`. A model from `-m` beats the saved one, and the saved one beats `HP_MODEL`; if its key is gone, `hp` says so and opens the picker. With no `-m`, no saved model and no `HP_MODEL`, it opens the picker first. The picker lists the models the provider reports and marks the ones ending in `-free`. `hp run`, `hp run -d` and `hp resume` ignore the saved model and use `-m`, then `HP_MODEL`. `hp providers` lists the presets and which keys are set.

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

The stripped binary is 8,724,743 bytes (8.7 MB), measured with `make check` against the 9,011,200 byte budget in `.size-budget`.

`make bench` runs hp against a local fake server that replies at once, so it checks hp's own overhead. It fails when any of these bounds is exceeded:

| Check | Bound |
| --- | --- |
| `hp --version` | 10 ms |
| `hp runs` with 500 journals | 100 ms |
| `hp run` start to first byte | 15 ms |
| Peak RSS | 40 MB |

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
internal/crew       crew mode: plan into parallel durable runs
internal/tui        terminal UI
internal/web        local web UI
tools/              dev tools used by checks, never shipped
```

## Credits

hp follows the shape of [fx](https://github.com/vercel-labs/fx) (Apache-2.0): its ask, auto and full-access permission modes, and its aim of keeping the binary small. Web search uses [Exa](https://exa.ai) when `EXA_API_KEY` is set. MCP servers speak the [Model Context Protocol](https://modelcontextprotocol.io). hp itself is licensed under [Apache 2.0](LICENSE).
