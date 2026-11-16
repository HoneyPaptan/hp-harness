package crew

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/HoneyPaptan/hp-harness/internal/host"
)

const (
	defaultVerifyTimeout = 10 * time.Minute
	verifyTailLines      = 60
	checkRunning         = "running"
	checkPassed          = "passed"
	checkFailed          = "failed"
)

type verifyJob struct {
	slug, command, dir, log string
	timeout                 time.Duration
	verdict                 string
}

type verifier struct {
	results chan verifyJob
	wake    chan struct{}
	running map[string]bool
}

func newVerifier(tasks int) *verifier {
	return &verifier{results: make(chan verifyJob, tasks), wake: make(chan struct{}, 1), running: map[string]bool{}}
}

func (v *verifier) sleep(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-v.wake:
		return nil
	case <-t.C:
		return nil
	}
}

func verifyLogPath(getenv func(string) string, id, slug string) string {
	return filepath.Join(crewsDir(getenv), id, slug+".verify.log")
}

func (s *State) verifyPending(t *Task) bool {
	return s.Verify != "" && t.Status == StatusFinished && (t.Check == "" || t.Check == checkRunning)
}

func (v *verifier) step(ctx context.Context, env Env, st *State) error {
	for done := false; !done; {
		select {
		case j := <-v.results:
			delete(v.running, j.slug)
			if t := st.task(j.slug); t != nil {
				t.Check = j.verdict
				if err := st.save(env.Getenv); err != nil {
					return err
				}
			}
		default:
			done = true
		}
	}
	for i := range st.Tasks {
		t := &st.Tasks[i]
		if !st.verifyPending(t) || v.running[t.Slug] {
			continue
		}
		t.Check = checkRunning
		if err := st.save(env.Getenv); err != nil {
			return err
		}
		v.running[t.Slug] = true
		go v.run(ctx, verifyJob{slug: t.Slug, command: st.Verify, dir: t.Worktree, log: verifyLogPath(env.Getenv, st.ID, t.Slug), timeout: st.VerifyTimeout})
	}
	return nil
}

func (v *verifier) run(ctx context.Context, job verifyJob) {
	if job.timeout <= 0 {
		job.timeout = defaultVerifyTimeout
	}
	res, err := host.Native{}.Exec(ctx, host.ExecRequest{Command: job.command, Dir: job.dir, Timeout: job.timeout})
	if ctx.Err() != nil {
		return
	}
	job.verdict = checkPassed
	tail := fmt.Sprintf("exit code: %d\n", res.ExitCode)
	switch {
	case err != nil:
		job.verdict, tail = checkFailed, err.Error()+"\n"
	case res.TimedOut:
		job.verdict, tail = checkFailed, fmt.Sprintf("timed out after %s\n", job.timeout)
	case res.ExitCode != 0:
		job.verdict = checkFailed
	}
	_ = os.WriteFile(job.log, []byte("$ "+job.command+"\n"+string(res.Output)+tail), 0o600)
	v.results <- job
	select {
	case v.wake <- struct{}{}:
	default:
	}
}

func verifyWord(t *Task) string {
	switch t.Check {
	case checkRunning:
		return "verifying"
	case checkPassed:
		return "verified"
	case checkFailed:
		return "verify failed"
	}
	return string(t.Status)
}

func verifyLines(env Env, st *State, t *Task) []string {
	if st.Verify == "" || t.Status != StatusFinished {
		return nil
	}
	lines := []string{verifyWord(t) + " · " + st.Verify}
	path := verifyLogPath(env.Getenv, st.ID, t.Slug)
	data, err := os.ReadFile(path)
	if err != nil {
		return lines
	}
	out := textLines(string(data))
	if len(out) > verifyTailLines {
		lines = append(lines, fmt.Sprintf("… %d earlier lines in %s", len(out)-verifyTailLines, path))
		out = out[len(out)-verifyTailLines:]
	}
	return append(lines, out...)
}
