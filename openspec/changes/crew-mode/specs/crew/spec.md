## ADDED Requirements

### Requirement: Run a crew from a plan

`hp crew run <plan.md>` SHALL split the plan into tasks, one per `## ` heading, and run each task as a detached hp run in its own git worktree on a new branch, at most `-n` at once.

#### Scenario: Plan split

- **WHEN** a plan has two `## ` headings with text under each
- **THEN** the crew has two tasks with those titles and prompts

#### Scenario: One worktree per task

- **WHEN** a task starts
- **THEN** it gets a worktree on branch `crew/<crew-id>/<slug>` forked from the repo's current HEAD, and its run has `-C` set to that worktree

### Requirement: Crew state is durable

A crew SHALL keep one state file that maps each task to its run id, branch, worktree and status, written before each transition, so `hp crew resume <crew-id>` continues after the hp process, the terminal or the machine dies.

#### Scenario: Resume after death

- **WHEN** the crew process dies with one task finished and one still running
- **THEN** `hp crew resume <crew-id>` keeps the finished task finished and waits for the running one

#### Scenario: Dead run resumes

- **WHEN** a task's child run process is gone and its journal is not finished
- **THEN** `hp crew resume <crew-id>` resumes it with `hp resume -d <run-id>`

#### Scenario: Never-started task

- **WHEN** a task has no run id recorded
- **THEN** resume starts it as if from a fresh run

### Requirement: Inspect a crew

`hp crew status <crew-id>` SHALL print one line per task with its title, state, run id, branch and files changed. `hp crew report <crew-id>` SHALL print each task's final answer and its diffstat against the fork point. `hp crews` SHALL list every crew.

#### Scenario: Status line

- **WHEN** a finished task changed two files in its worktree
- **THEN** its status line names the title, `finished`, the run id, the branch and `2 files`

#### Scenario: Report

- **WHEN** a finished task produced a final answer
- **THEN** `hp crew report` prints that answer and the task's diffstat
