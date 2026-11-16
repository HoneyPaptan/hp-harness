package crew

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type landFixture struct {
	svc    Service
	getenv func(string) string
	repo   string
	branch string
	spawns *[]spawnCall
	st     *State
}

func newLandFixture(t *testing.T, status Status) *landFixture {
	t.Helper()
	repo := initRepo(t)
	getenv := getenvMap(map[string]string{"XDG_STATE_HOME": t.TempDir(), "HOME": "/h"})
	var spawns []spawnCall
	env := Env{
		Getenv: getenv, Cwd: repo, Alive: func(int) bool { return false },
		Spawn: func(args []string, log string) (int, error) {
			spawns = append(spawns, spawnCall{args, log})
			return 4242, nil
		},
	}
	root, head, err := gitRootHead(env, repo)
	if err != nil {
		t.Fatal(err)
	}
	st := newState(getenv, "c1", "/p/plan.md", root, head, runFlags{n: 1}, []planTask{{title: "add b file", prompt: "p", slug: "b"}})
	st.Tasks[0].Status = status
	runGit(t, repo, "worktree", "add", "-q", "-b", st.Tasks[0].Branch, st.Tasks[0].Worktree, head)
	if err := st.save(getenv); err != nil {
		t.Fatal(err)
	}
	return &landFixture{svc: Service{Env: env}, getenv: getenv, repo: repo, branch: strings.TrimSpace(runGit(t, repo, "symbolic-ref", "--short", "HEAD")), spawns: &spawns, st: st}
}

func (f *landFixture) write(t *testing.T, dir, name, text string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

func (f *landFixture) task() *Task { return &f.st.Tasks[0] }

func (f *landFixture) head(t *testing.T) string {
	t.Helper()
	return strings.TrimSpace(runGit(t, f.repo, "rev-parse", "HEAD"))
}

func (f *landFixture) queued() []request {
	var out []request
	for _, q := range queuedRequests(f.getenv, "c1") {
		out = append(out, q.req)
	}
	return out
}

func TestLandCommitsTheWorktreeAndMergesWithAMergeCommit(t *testing.T) {
	f := newLandFixture(t, StatusFinished)
	f.write(t, f.task().Worktree, "a.txt", "a changed\n")
	f.write(t, f.task().Worktree, "b.txt", "new file\n")
	before := f.head(t)
	question, err := f.svc.LandCheck("c1", "b")
	if err != nil || question != "merge crew/c1/b into "+f.branch+"?" {
		t.Fatalf("question %q %v", question, err)
	}
	msg, err := f.svc.Land("c1", "b")
	if err != nil || msg != "landed crew/c1/b into "+f.branch {
		t.Fatalf("land %q %v", msg, err)
	}
	if parents := strings.Fields(runGit(t, f.repo, "log", "-1", "--format=%P")); len(parents) != 2 || parents[0] != before {
		t.Fatalf("head is not a merge on top of the old head: %v", parents)
	}
	if got, _ := os.ReadFile(filepath.Join(f.repo, "b.txt")); string(got) != "new file\n" {
		t.Fatalf("b.txt %q", got)
	}
	if got, _ := os.ReadFile(filepath.Join(f.repo, "a.txt")); string(got) != "a changed\n" {
		t.Fatalf("a.txt %q", got)
	}
	taskCommit := runGit(t, f.repo, "log", "-1", "--format=%an <%ae>%n%B", f.task().Branch)
	if strings.TrimSpace(taskCommit) != "t <t@example.com>\nadd b file" {
		t.Fatalf("task commit %q", taskCommit)
	}
	if body := runGit(t, f.repo, "log", "--format=%B"); strings.Contains(strings.ToLower(body), "co-authored") || strings.Contains(strings.ToLower(body), "generated") {
		t.Fatalf("attribution in history: %q", body)
	}
	if reqs := f.queued(); len(reqs) != 1 || reqs[0].Op != opLanded || reqs[0].Task != "b" || len(*f.spawns) != 1 {
		t.Fatalf("queued %v spawns %v", reqs, *f.spawns)
	}
	if err := applyControls(f.svc.Env, f.st); err != nil {
		t.Fatal(err)
	}
	if !f.task().Landed || statusWord(f.task()) != "landed" || taskKeys(f.task()) != "dx" {
		t.Fatalf("task %+v keys %q", f.task(), taskKeys(f.task()))
	}
}

func TestLandAlreadyCommittedWorkNeedsNoNewCommit(t *testing.T) {
	f := newLandFixture(t, StatusFinished)
	f.write(t, f.task().Worktree, "b.txt", "x\n")
	runGit(t, f.task().Worktree, "add", "-A")
	runGit(t, f.task().Worktree, "commit", "-q", "-m", "the agent committed")
	if _, err := f.svc.Land("c1", "b"); err != nil {
		t.Fatal(err)
	}
	if log := runGit(t, f.repo, "log", "--format=%s", f.task().Branch); !strings.HasPrefix(log, "the agent committed") {
		t.Fatalf("log %q", log)
	}
}

func TestLandRefusals(t *testing.T) {
	cases := []struct {
		name  string
		setup func(*testing.T, *landFixture)
		want  string
	}{
		{"uncommitted changes in the repository", func(t *testing.T, f *landFixture) {
			f.write(t, f.repo, "a.txt", "edited by hand\n")
		}, "uncommitted changes"},
		{"detached HEAD", func(t *testing.T, f *landFixture) { runGit(t, f.repo, "checkout", "-q", "--detach") }, "detached HEAD"},
		{"task still running", func(t *testing.T, f *landFixture) { f.task().Status = StatusRunning }, "cannot be landed now"},
		{"task failed", func(t *testing.T, f *landFixture) { f.task().Status = StatusFailed }, "cannot be landed now"},
		{"verify still running", func(t *testing.T, f *landFixture) { f.task().Check = checkRunning }, "cannot be landed now"},
		{"already landed", func(t *testing.T, f *landFixture) { f.task().Landed = true }, "cannot be landed now"},
		{"discarded", func(t *testing.T, f *landFixture) { f.task().Discarded = true }, "cannot be landed now"},
		{"nothing changed", func(t *testing.T, f *landFixture) { f.write(t, f.repo, "ignored", "") }, "changed nothing"},
		{"request already waiting", func(t *testing.T, f *landFixture) {
			if err := enqueue(f.getenv, "c1", request{Op: opLanded, Task: "b"}); err != nil {
				t.Fatal(err)
			}
		}, "already has a request waiting"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newLandFixture(t, StatusFinished)
			if tc.name != "nothing changed" {
				f.write(t, f.task().Worktree, "b.txt", "x\n")
			}
			tc.setup(t, f)
			if err := f.st.save(f.getenv); err != nil {
				t.Fatal(err)
			}
			before, status := f.head(t), runGit(t, f.repo, "status", "--porcelain")
			_, err := f.svc.Land("c1", "b")
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err %v want %q", err, tc.want)
			}
			if f.head(t) != before || runGit(t, f.repo, "status", "--porcelain") != status {
				t.Fatal("the repository changed")
			}
			for _, req := range f.queued() {
				if req.Op == opLanded {
					if tc.name != "request already waiting" {
						t.Fatalf("recorded a landing that did not happen: %v", f.queued())
					}
				}
			}
		})
	}
}

func TestLandConflictLeavesTheRepositoryAsItWas(t *testing.T) {
	f := newLandFixture(t, StatusFinished)
	f.write(t, f.task().Worktree, "a.txt", "from the task\n")
	f.write(t, f.repo, "a.txt", "from the user\n")
	runGit(t, f.repo, "commit", "-q", "-am", "user edit")
	before := f.head(t)
	_, err := f.svc.Land("c1", "b")
	if err == nil || !strings.Contains(err.Error(), "conflicts in a.txt") || !strings.Contains(err.Error(), "left as it was") {
		t.Fatalf("err %v", err)
	}
	if f.head(t) != before {
		t.Fatal("head moved")
	}
	if status := runGit(t, f.repo, "status", "--porcelain"); status != "" {
		t.Fatalf("status %q", status)
	}
	if _, err := os.Stat(filepath.Join(f.repo, ".git", "MERGE_HEAD")); err == nil {
		t.Fatal("merge left in progress")
	}
	if got, _ := os.ReadFile(filepath.Join(f.repo, "a.txt")); string(got) != "from the user\n" {
		t.Fatalf("a.txt %q", got)
	}
	if len(f.queued()) != 0 {
		t.Fatalf("queued %v", f.queued())
	}
}

func TestDiscardRemovesWorktreeAndBranch(t *testing.T) {
	for _, status := range []Status{StatusFinished, StatusFailed, StatusStopped} {
		t.Run(string(status), func(t *testing.T) {
			f := newLandFixture(t, status)
			f.write(t, f.task().Worktree, "b.txt", "x\n")
			if err := f.svc.Discard("c1", "b"); err != nil {
				t.Fatal(err)
			}
			if _, err := os.Stat(f.task().Worktree); err == nil {
				t.Fatal("worktree still there")
			}
			if branches := runGit(t, f.repo, "branch", "--list", f.task().Branch); strings.TrimSpace(branches) != "" {
				t.Fatalf("branch still there: %q", branches)
			}
			if reqs := f.queued(); len(reqs) != 1 || reqs[0].Op != opDiscarded {
				t.Fatalf("queued %v", reqs)
			}
			if err := applyControls(f.svc.Env, f.st); err != nil {
				t.Fatal(err)
			}
			if statusWord(f.task()) != "discarded" || taskKeys(f.task()) != "" || changedFiles(f.svc.Env, f.st, f.task()) != "-" {
				t.Fatalf("task %+v keys %q", f.task(), taskKeys(f.task()))
			}
		})
	}
}

func TestDiscardRefusals(t *testing.T) {
	cases := []struct {
		name   string
		status Status
		edit   func(*Task)
	}{
		{"running", StatusRunning, func(*Task) {}},
		{"queued", StatusQueued, func(*Task) {}},
		{"verifying", StatusFinished, func(t *Task) { t.Check = checkRunning }},
		{"already discarded", StatusFinished, func(t *Task) { t.Discarded = true }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newLandFixture(t, tc.status)
			tc.edit(f.task())
			if err := f.st.save(f.getenv); err != nil {
				t.Fatal(err)
			}
			if err := f.svc.Discard("c1", "b"); err == nil || !strings.Contains(err.Error(), "cannot be discarded now") {
				t.Fatalf("err %v", err)
			}
			if _, err := os.Stat(f.task().Worktree); err != nil {
				t.Fatal("worktree was removed")
			}
		})
	}
}

func TestDiscardTaskThatNeverStarted(t *testing.T) {
	f := newLandFixture(t, StatusStopped)
	runGit(t, f.repo, "worktree", "remove", "--force", f.task().Worktree)
	runGit(t, f.repo, "branch", "-D", f.task().Branch)
	if err := f.svc.Discard("c1", "b"); err != nil {
		t.Fatal(err)
	}
}

func TestDiffShowsCommittedChangedAndNewFiles(t *testing.T) {
	f := newLandFixture(t, StatusFinished)
	f.write(t, f.task().Worktree, "a.txt", "a changed\n")
	runGit(t, f.task().Worktree, "commit", "-q", "-am", "committed by the agent")
	f.write(t, f.task().Worktree, "c.txt", "uncommitted edit\n")
	runGit(t, f.task().Worktree, "add", "c.txt")
	f.write(t, f.task().Worktree, "d.txt", "brand new\n")
	lines, err := f.svc.Diff("c1", "b")
	if err != nil {
		t.Fatal(err)
	}
	text := strings.Join(lines, "\n")
	for _, want := range []string{"+a changed", "+uncommitted edit", "+brand new", "d.txt"} {
		if !strings.Contains(text, want) {
			t.Fatalf("diff lacks %q:\n%s", want, text)
		}
	}
}

func TestDiffWithNothingAndWithoutAWorktree(t *testing.T) {
	f := newLandFixture(t, StatusFinished)
	lines, err := f.svc.Diff("c1", "b")
	if err != nil || len(lines) != 1 || lines[0] != "(no changes)" {
		t.Fatalf("lines %v %v", lines, err)
	}
	runGit(t, f.repo, "worktree", "remove", "--force", f.task().Worktree)
	if _, err := f.svc.Diff("c1", "b"); err == nil || !strings.Contains(err.Error(), "no worktree") {
		t.Fatalf("err %v", err)
	}
	f.task().Status = StatusRunning
	if err := f.st.save(f.getenv); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.Diff("c1", "b"); err == nil {
		t.Fatal("a running task has no finished diff")
	}
}

func TestViewKeysForReviewStates(t *testing.T) {
	cases := []struct {
		name string
		edit func(*Task)
		keys string
		word string
	}{
		{"finished", func(*Task) {}, "dax", "finished"},
		{"verifying", func(t *Task) { t.Check = checkRunning }, "d", "verifying"},
		{"failed", func(t *Task) { t.Status = StatusFailed }, "rdx", "failed"},
		{"stopped", func(t *Task) { t.Status = StatusStopped }, "rdx", "stopped"},
		{"running", func(t *Task) { t.Status = StatusRunning }, "s", "running"},
		{"landed", func(t *Task) { t.Landed = true }, "dx", "landed"},
		{"discarded", func(t *Task) { t.Discarded = true }, "", "discarded"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newLandFixture(t, StatusFinished)
			tc.edit(f.task())
			if err := f.st.save(f.getenv); err != nil {
				t.Fatal(err)
			}
			snap, _, err := f.svc.View("c1")
			if err != nil {
				t.Fatal(err)
			}
			if snap.Rows[0].Keys != tc.keys || snap.Rows[0].Status != tc.word {
				t.Fatalf("row %+v", snap.Rows[0])
			}
		})
	}
}

func TestViewShowsLandingAndDiscardAtOnce(t *testing.T) {
	f := newLandFixture(t, StatusFinished)
	if err := enqueue(f.getenv, "c1", request{Op: opLanded, Task: "b"}); err != nil {
		t.Fatal(err)
	}
	snap, _, _ := f.svc.View("c1")
	if snap.Rows[0].Status != "landed" || snap.Rows[0].Keys != "" || snap.Done {
		t.Fatalf("row %+v done %v", snap.Rows[0], snap.Done)
	}
}

func TestFollowOfADiscardedTaskSaysSo(t *testing.T) {
	f := newLandFixture(t, StatusFinished)
	f.task().RunID = "r1"
	seedEnd(t, f.getenv, "r1", "final")
	if err := f.st.save(f.getenv); err != nil {
		t.Fatal(err)
	}
	if err := f.svc.Discard("c1", "b"); err != nil {
		t.Fatal(err)
	}
	if err := applyControls(f.svc.Env, f.st); err != nil {
		t.Fatal(err)
	}
	chunk, err := f.svc.Follow("c1", 0, 0)
	if err != nil || len(chunk.Trailer) != 1 || !strings.HasPrefix(chunk.Trailer[0], "discarded") {
		t.Fatalf("trailer %v %v", chunk.Trailer, err)
	}
}
