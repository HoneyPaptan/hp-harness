## Why

hp makes one run survive anything. Crew mode applies that to several runs at once: `hp crew run <plan.md>` splits a markdown plan into tasks, runs each task as its own detached hp run in its own git worktree, a few at a time, and stays resumable across a dead terminal, SSH loss or reboot. A plan is one file, so a lead can hand it to hp and let it fan the work out to workers without a second engine.

## What Changes

- `hp crew run <plan.md>` with `-n` (tasks at once, default 2) and `-m`, `--base-url`, `--key-env` exactly as on `hp run`. Each `## <title>` heading in the plan is one task and the text under it is that task's prompt. Each task gets a git worktree on a new branch `crew/<crew-id>/<task-slug>` forked from the repo's current HEAD, then runs as a normal detached hp run with `-C` set to the worktree.
- One state file per crew under hp's state directory (`~/.local/state/hp/crews/<id>.json`) maps each task to its run id, branch, worktree and status, and is written before each transition, the same ordering the journal uses. `hp crew resume <crew-id>` continues: finished tasks stay finished, dead runs resume through `hp resume -d`, and tasks that never started start then.
- `hp crew status <crew-id>` prints one line per task (title, state, run id, branch, files changed). `hp crew report <crew-id>` prints each task's final answer and its diffstat against the fork point. `hp crews` lists crews.

## Non-goals

- A lead model that writes the plan or reviews the results.
- Merging branches, removing worktrees, or a crew of more than one repo.
- Any change to the web UI or the wasm build.

## Binary size

About 30 KB of new code. Crew mode is wired only into `cmd/hp`, so the wasm build does not include it and its gzip size is unchanged.
