# hp

A small, fast coding agent for the terminal, built so a run never dies.

> Early work in progress. Runs, journals, resumes, waits out rate limits, falls back to other models and runs in the background today. Reboot survival, a TUI and `hp add` are next.

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

Requires Go 1.27 or newer.

## Use

```
export OPENCODE_API_KEY=...
./hp run -m opencode/grok-4.6 "find the TODOs in this repo and fix the easy ones"
./hp runs
./hp resume <run-id>
```

`-m` takes `preset/model`. Presets:

| Preset | Endpoint | Key |
| --- | --- | --- |
| `opencode` | `https://opencode.ai/inference/openai/v1` | `OPENCODE_API_KEY` |
| `xai` | `https://api.x.ai/v1` | `XAI_API_KEY` |
| `openrouter` | `https://openrouter.ai/api/v1` | `OPENROUTER_API_KEY` |
| `groq` | `https://api.groq.com/openai/v1` | `GROQ_API_KEY` |
| `ollama` | `http://127.0.0.1:11434/v1` | none |

Any other OpenAI compatible server works with `--base-url` and `--key-env`. `HP_MODEL` sets a default model.

Text streams to stdout, tool calls print one line each to stderr, and a summary line ends the run. Press Ctrl-C at any time: the run stays resumable. Journals live in `~/.local/state/hp/runs`.

Built-in tools: `read`, `write`, `edit`, `bash`, `grep`. Skills are folders with a `SKILL.md` in `.hp/skills`, `.claude/skills` or `~/.config/hp/skills`; the model sees their names and loads one with the `skill` tool when it needs it.

## Limits and background runs

```
./hp run -d -m groq/openai/gpt-oss-120b --fallback groq/openai/gpt-oss-20b "refactor the parser"
./hp attach <run-id>
./hp stop <run-id>
```

On a rate limit (429) or overload (503, 529) hp reads the reset time from `Retry-After` or the `x-ratelimit-reset-*` headers, prints when it will continue, waits and retries the same step. If the wait is longer than `--switch-after` (default 20s) and a `--fallback` model is free, it switches at once instead. `--max-wait` (default 6h) caps one wait; past it hp stops with exit code 4 and the run stays resumable. Waits and switches are journaled, so `hp resume` keeps the model the run was on.

`-d` on `run` or `resume` starts the run in its own session and returns. Output goes to `<run-id>.log` next to the journal. `hp attach` prints it and follows along; Ctrl-C detaches and leaves the run going. `hp runs` shows live runs as `running`, and a second `hp resume` on a live run is refused.

## License

Apache 2.0
