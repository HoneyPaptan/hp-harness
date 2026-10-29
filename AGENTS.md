# AGENTS.md

hp is a minimal, fast coding agent harness in Go. Its headline feature is a run that never dies: every step is journaled, and a run survives usage limits, closed terminals, lost SSH and reboots, then resumes from the last completed step.

## Layout

- `cmd/hp`: entry point, wires flags to packages, nothing else
- `internal/core`: the agent loop; talks to the world only through `Host`
- `internal/host`: the `Host` interface and its native implementation
- `internal/journal`: append-only step log, replay and resume
- `internal/provider`: one adapter per model API, streaming only
- `internal/tool`: built-in tools, one file each
- `internal/tui`: terminal UI
- `tools/`: dev tools used by checks, never shipped

Create a package only when the first code for it lands.

## Rules

- No comments in Go code. `//go:` directives are the only exception. Names carry the meaning.
- Small named functions and small files. Extract shared logic on the second caller.
- `internal/core` never imports `os`, `os/exec`, `net`, `net/http` or `syscall`.
- Standard library first. A new dependency needs a reason and its binary size cost in the PR.
- The stripped binary must stay under `.size-budget` bytes.
- Tests sit next to the code, table driven, no network and no real model calls. Use a fake provider.
- One logical change per commit, clear message, no AI attribution lines.

## Before a PR

```
make test
make check
```

`make check` runs gofmt, go vet, the no comments check, the core boundary check and the size budget.
