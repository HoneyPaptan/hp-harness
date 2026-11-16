## 1. Plan and state

- [ ] 1.1 `internal/crew`: split a markdown plan into tasks (`## ` headings are titles, the text under each is its prompt) and slug/branch/worktree naming. Verify with table tests on plan files and slug edge cases.
- [ ] 1.2 `internal/crew`: crew state struct, JSON load and save, state and worktree directory paths, crew id. Verify with table tests over the memory host.

## 2. Git

- [ ] 2.1 `internal/crew`: resolve the repo root and HEAD, create a worktree and branch, diffstat and files-changed against the fork point. Verify with a temp git repo (init, commit, worktree, edit, diff).

## 3. Supervise

- [ ] 3.1 `internal/crew`: `crew run` and `crew resume` share one supervise loop that starts queued tasks up to `-n`, marks finished tasks from the child journal, and resumes dead runs through `hp resume -d`. The `hp` spawn is a seam, faked in tests. Verify with table tests that drive queued to running to finished and a dead run to resumed.

## 4. Commands

- [ ] 4.1 `internal/crew`: `status`, `report` and `crews` rendering from a state file and real journals. Verify with a temp state dir holding a finished and a queued task.
- [ ] 4.2 `cmd/hp`: intercept `crew` and `crews` before `cli.Main` and wire `crew.Main`. Verify the commands reach `crew` and the wasm build still compiles without crew.

## 5. Check

- [ ] 5.1 README section, `make test`, `make check` and `make bench` at the head. Real test: a scratch repo with two small tasks, `hp crew run` on a local mock provider, kill the crew process mid-run, `hp crew resume`, then `hp crew status` and `hp crew report`, with the output in the PR body.
