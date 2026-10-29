# hp

A small, fast coding agent for the terminal, built so a run never dies.

> Early work in progress. Runs, journals and resumes today; limit waits, the background daemon and `hp attach` are next.

## Why

Coding agents stop when you hit a usage limit, close the laptop or lose SSH. hp keeps going.

- **Survives limits.** When a provider says you are out of quota, hp reads the reset time, waits, and continues from the same step. Or it falls back to another model you allowed.
- **Survives you leaving.** Close the terminal or reboot; the run carries on in the background. `hp attach` puts you back in.
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
| `ollama` | `http://127.0.0.1:11434/v1` | none |

Any other OpenAI compatible server works with `--base-url` and `--key-env`. `HP_MODEL` sets a default model.

Text streams to stdout, tool calls print one line each to stderr, and a summary line ends the run. Press Ctrl-C at any time: the run stays resumable. Journals live in `~/.local/state/hp/runs`.

Built-in tools: `read`, `write`, `edit`, `bash`, `grep`. Skills are folders with a `SKILL.md` in `.hp/skills`, `.claude/skills` or `~/.config/hp/skills`; the model sees their names and loads one with the `skill` tool when it needs it.

## License

Apache 2.0
