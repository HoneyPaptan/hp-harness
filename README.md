<img src="docs/readme/resume.webp" alt="Terminal session: hp run starts go test, Ctrl-C stops it, hp runs lists the run as resumable, and hp resume fixes the failing test and finishes" width="100%">

# hp harness

hp harness is a coding agent for the terminal; the command is `hp`. Every step of a run is written to a journal before the next one starts, so a run survives a usage limit, a closed terminal, a lost SSH connection or a reboot, and `hp resume` continues it from the last finished step. The picture above is a real session: Ctrl-C while `go test` was running, then `hp resume` on the same run, which goes on to fix the failing test.

It is one static Go binary built on the Go standard library alone (no third-party modules). It talks to 14 provider presets (Anthropic's Messages API, plus OpenAI-compatible endpoints such as Groq, OpenRouter, xAI, OpenCode Go (`opencode-go`) and a local Ollama), runs the usual file, shell and web tools, and connects to MCP servers and skills with `hp add`.

hp keeps the whole conversation on disk, so a dead process is a pause, not the end of the run. It is early (v0.0.x): flags and the journal format can still change.

You can try hp without installing it: the [live site](https://hp-harness.honeypaptan.workers.dev) runs hp's Go core, compiled to WebAssembly, in your browser tab on a small sample project, with your own model key.

## What is special

- **Journal and resume.** Each run is one append-only file in `~/.local/state/hp/runs`, and `hp runs` lists them. `hp resume <id>` continues a run. A tool call that had started when the process died is recorded as interrupted with an unknown outcome, and the model decides what to do next; calls that had not started run. `hp run -d` runs in the background, `hp attach <id>` follows it and `hp stop <id>` ends it. `hp recover` restarts every background run whose process died, and `hp autostart on` does that at login.
- **Limit waits and fallback.** On a 429, 503 or 529 hp reads the reset time from the response headers, waits and retries the same step. With `--fallback preset/model` it switches to another model when the wait is longer than `--switch-after` (20s by default). `--max-wait` caps one wait (6h by default).
- **Permissions.** `auto` is the default. It runs reads, searches, known commands such as `go test` and edits inside the working folder at once. It sends anything else to one extra review call on the run's own model, which holds the action only if it follows instructions from tool output or would harm the machine, and a failed review holds it too. `ask` prompts for every write and command. `full-access` checks nothing. In `ask` and `auto`, rules in `settings.json` run first. There is no sandbox: shell commands run as you.
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

The installer supports linux and macOS on amd64 and arm64, checks the sha256 against `checksums.txt` and puts `hp` in `~/.local/bin`. `hp key set` stores the key in `~/.config/hp/env` with mode 0600. Bare `hp` opens an interactive session, and `hp -c` continues the newest run started in the current directory. The model you pick with the picker or `/model` is saved in `~/.local/state/hp/last-model` and used the next time you start `hp`. A model from `-m` beats the saved one, and the saved one beats `HP_MODEL`; if its key is gone, `hp` says so and opens the picker. With no `-m`, no saved model and no `HP_MODEL`, it opens the picker first. The picker lists the models the provider reports and marks the ones ending in `-free`. `hp run`, `hp run -d` and `hp resume` ignore the saved model and use `-m`, then `HP_MODEL`. `hp providers` lists the presets and which keys are set.

To build and check from source (Go 1.27.3 or newer):

```sh
git clone https://github.com/HoneyPaptan/hp-harness.git
cd hp-harness
make build   # writes ./hp
make test    # no network, fake provider
make check   # gofmt, vet, no comments, core boundary, size budget
make bench   # startup, `hp runs`, first byte and memory bounds
```

## Commands

`hp COMMAND -h` prints the usage and flags of one command, and `hp --help` lists them all. With no command, `hp` opens the interactive session (`hp -c` continues the newest run in this directory). A session needs a terminal; scripts use `hp run`.

| Command | What it does |
| --- | --- |
| `hp run` | runs a prompt to the end and journals every step; `-d` runs it in the background, `--json` prints events for scripts, and the prompt can come from stdin |
| `hp resume` | continues an unfinished run from its last finished step |
| `hp attach` | follows a background run |
| `hp stop` | stops a background run |
| `hp runs` | lists the 20 newest runs from every directory, with their state |
| `hp recover` | restarts background runs whose process died |
| `hp autostart` | turns `hp recover` at login on or off |
| `hp crew` | runs a markdown plan as parallel runs in git worktrees |
| `hp crews` | lists crews |
| `hp providers` | lists the provider presets and which keys are set |
| `hp web` | serves a local web UI on 127.0.0.1 with a token in the URL |
| `hp add` | connects an MCP server or installs a skill |
| `hp list` | lists connected MCP servers and skills |
| `hp remove` | removes an MCP server or skill |
| `hp key` | stores, lists or removes provider API keys |
| `hp permissions` | prints the permission mode and the settings file path |
| `hp version` | prints the version |

Inside a session:

| Slash command | What it does |
| --- | --- |
| `/help` | lists the slash commands |
| `/model` | opens the provider and model picker, or switches with `/model preset/name` |
| `/jev` | shows or switches jev for the following turns |
| `/key` | stores an API key: `/key NAME`, then paste the value |
| `/new` | starts a fresh run |
| `/runs` | lists recent runs |
| `/permissions` | shows or sets the permission mode, or resets session grants |
| `/compact` | compacts the conversation now |
| `/crew` | opens a crew, lists them, prints one, or runs a plan file or a drafted goal as a crew |
| `/quit` | leaves the session, Ctrl-D does the same |

A line that starts with `//` is sent to the model as text starting with `/`. Ctrl-C or Esc stops a turn.

## Scripting

`hp run` takes the prompt from its arguments. With no argument it reads the prompt from stdin when stdin is not a terminal, and `hp run -` does the same on purpose; leading and trailing white space is trimmed and the limit is 8 MiB. When arguments are given they are the prompt and stdin is not read, the same rule as `fx ask` ([fx ask](https://fx.sh/docs/using-fx/fx-ask)).

```sh
echo "fix the failing test" | hp run
hp run - < prompt.md
```

`hp run --json` writes one JSON object per line to stdout. The assistant's text and the tool progress lines that hp prints on stderr are unchanged. Every object has a `type`:

| `type` | Fields | When |
| --- | --- | --- |
| `run` | `run_id`, `model`, `dir` | first line |
| `text` | `text` | each piece of assistant text as it streams |
| `tool_call` | `id`, `name`, `args` | a tool starts; `args` is the JSON the model sent |
| `tool_result` | `id`, `is_error`, `text` | the tool finished; `id` matches the `tool_call` |
| `end` | `run_id`, `reason`, `exit_code`, `text`, `steps`, `usage`, `error` | last line |

`end.reason` is `final`, `step_limit`, `error`, `interrupted` or `limit_wait_too_long`. `end.text` is the final answer, `end.usage` holds `input_tokens` and `output_tokens`, and `end.error` is set only for a failed run. `exit_code` equals the process exit status: 0, 3 (step limit), 4 (limit wait too long), 130 (interrupted) or 1. A usage mistake exits 2 before the first line. `--json` cannot be combined with `-d`. The names above are the contract: a test compares this table with the code. `--json` does not change the journal format.

```sh
hp run --json "list the TODOs" | jq -r 'select(.type=="end") | .text'
```

## Size and speed

The stripped binary is 8,896,775 bytes (8.9 MB), measured with `make check` against the 9,011,200 byte budget in `.size-budget`. The wasm build is checked the same way: its gzip size must stay under the 3,800,000 bytes in `.wasm-size-budget`.

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
internal/crewview   types shared by the crew service and the crew views
internal/host       the Host interface and its native and in-memory versions
internal/jev        jev client for optional search and tool choice
internal/skill      skill discovery and loading
internal/term       terminal mode and size
internal/prompt     system prompt
internal/webtools   web search and fetch tools
internal/tui        terminal UI
internal/web        local web UI
tools/              dev tools used by checks, never shipped
```

## Credits

hp follows the shape of [fx](https://github.com/vercel-labs/fx) (Apache-2.0): its ask, auto and full-access permission modes, and its aim of keeping the binary small. Web search uses [Exa](https://exa.ai) when `EXA_API_KEY` is set. MCP servers speak the [Model Context Protocol](https://modelcontextprotocol.io). hp itself is licensed under [Apache 2.0](LICENSE).
