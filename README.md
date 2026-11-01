# hp

A small, fast coding agent for the terminal, built so a run never dies.

> Early work in progress. Runs, journals, resumes, waits out rate limits, falls back to other models and runs in the background today, bare `hp` opens an inline session, and `hp add` connects MCP servers and skills. Reboot survival is next.

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
./hp -m opencode/grok-4.6
./hp run -m opencode/grok-4.6 "find the TODOs in this repo and fix the easy ones"
./hp runs
./hp resume <run-id>
```

`-m` takes `preset/model`. Presets:

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

Built-in tools: `read`, `write`, `edit`, `bash`, `grep`, plus `search` with `--jev`. Skills are folders with a `SKILL.md` in `.hp/skills`, `.claude/skills` or `~/.config/hp/skills`; the model sees their names and loads one with the `skill` tool when it needs it. MCP servers are added with `hp add`.

## Session

Bare `hp` on a terminal opens a session. The conversation prints inline into normal scrollback, with one dim status line after each turn. Every follow-up continues the same journaled run, so a session survives like any other run.

```
./hp -m opencode/grok-4.6
./hp -c
```

`-c` continues the latest run in this directory, and finishes it first if it stopped midway. `-m`, `-C`, `--jev`, `--base-url` and `--key-env` work here too. Without a terminal, bare `hp` prints usage. A run in a live session shows as `running`, and a second session or `hp resume` on it is refused.

Ctrl-C or Esc stops a turn and returns to the prompt; keys typed during a turn wait there as the next draft. At the prompt, Ctrl-C clears the text, twice on an empty prompt quits, and so does Ctrl-D.

Commands stay local and send nothing to the model: `/help`, `/model` shows the model, `/model preset/name` switches it for following turns, `/new` starts a fresh run, `/runs`, `/quit`. `//text` sends a literal `/text`.

A session opens its connection to the provider at start, and again on the first key after 60 seconds idle, so the first turn skips the TCP and TLS handshake. `hp web` does the same at page load. The probe is an unauthenticated `HEAD` on the base URL; if it fails, nothing shows and the turn connects as usual. On models that stream reasoning the activity line reads `thinking…`, time to first token stops there, and reasoning is neither printed nor journaled.

`NO_COLOR` or `TERM=dumb` turns colour off.

## Web

```
./hp web -m groq/openai/gpt-oss-120b
```

`hp web` serves a page from the binary and prints a URL with a one-time token to stderr. A run started there uses the same journal as `hp run`: closing the tab leaves it going, reopening replays the journal and rejoins live, and stopping the server leaves it resumable. Runs live in another process are shown but refused.

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

## Limits and background runs

```
./hp run -d -m groq/openai/gpt-oss-120b --fallback groq/openai/gpt-oss-20b "refactor the parser"
./hp attach <run-id>
./hp stop <run-id>
```

On a rate limit (429) or overload (503, 529) hp reads the reset time from `Retry-After` or the `x-ratelimit-reset-*` and `anthropic-ratelimit-*-reset` headers, prints when it will continue, waits and retries the same step. If the wait is longer than `--switch-after` (default 20s) and a `--fallback` model is free, it switches at once instead. `--max-wait` (default 6h) caps one wait; past it hp stops with exit code 4 and the run stays resumable. Waits and switches are journaled, so `hp resume` keeps the model the run was on.

`-d` on `run` or `resume` starts the run in its own session and returns. Output goes to `<run-id>.log` next to the journal. `hp attach` prints it and follows along; Ctrl-C detaches and leaves the run going. `hp runs` shows live runs as `running`, and a second `hp resume` on a live run is refused.

## License

Apache 2.0
