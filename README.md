# hp

A small, fast coding agent for the terminal, built so a run never dies.

> Early work in progress. Not usable yet.

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
./hp
```

Requires Go 1.27 or newer.

## License

Apache 2.0
