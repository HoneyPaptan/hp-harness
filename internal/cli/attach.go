package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"time"

	"github.com/HoneyPaptan/hp-harness/internal/core"
	"github.com/HoneyPaptan/hp-harness/internal/journal"
)

func cmdAttach(ctx context.Context, env Env, args []string) int {
	if len(args) != 1 {
		return exitUsage(env, usagef("run id required"))
	}
	id := args[0]
	jpath := filepath.Join(runsDir(env.Getenv), id+".jsonl")
	if _, err := journal.ReadStart(env.Host, jpath); err != nil {
		return exitUsage(env, usagef("run %q not found", id))
	}
	logPath := runLogPath(env.Getenv, id)
	offset := 0
	printLog := func() {
		data, err := env.Host.ReadFile(logPath)
		if err != nil {
			return
		}
		if len(data) <= offset {
			return
		}
		_, _ = io.WriteString(env.Stdout, string(data[offset:]))
		offset = len(data)
	}
	printLog()
	for {
		st, err := replayRun(env, jpath)
		if err != nil {
			fmt.Fprintf(env.Stderr, "error: %v\n", err)
			return 1
		}
		alive := runAlive(env, id)
		if st.Finished && !alive {
			printLog()
			return attachExit(st)
		}
		if !st.Finished && !alive {
			printLog()
			fmt.Fprintf(env.Stderr, "run %s stopped, resume with: hp resume %s\n", id, id)
			return 1
		}
		if err := env.Host.Sleep(ctx, 200*time.Millisecond); err != nil {
			if errors.Is(err, context.Canceled) || ctx.Err() != nil {
				fmt.Fprintf(env.Stderr, "detached, the run keeps going\n")
				return 130
			}
			fmt.Fprintf(env.Stderr, "error: %v\n", err)
			return 1
		}
		printLog()
	}
}

func replayRun(env Env, jpath string) (journal.State, error) {
	recs, err := journal.Read(env.Host, jpath)
	if err != nil {
		return journal.State{}, err
	}
	return journal.Replay(recs)
}

func attachExit(st journal.State) int {
	if st.EndReason == core.ReasonStepLimit {
		return 3
	}
	return 0
}
