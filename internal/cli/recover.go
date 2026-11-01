package cli

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"path/filepath"
	"time"

	"github.com/HoneyPaptan/hp-harness/internal/journal"
)

const (
	defaultMaxAge = 7 * 24 * time.Hour
	crashLimit    = 3
)

type recoverOpts struct {
	dryRun bool
	maxAge time.Duration
}

func cmdRecover(ctx context.Context, env Env, args []string) int {
	o, err := parseRecoverFlags(env, args)
	if err != nil {
		return exitUsage(env, err)
	}
	unlock, lockErr := env.Lock(recoverLockPath(env.Getenv))
	if lockErr != nil {
		fmt.Fprintln(env.Stderr, "hp recover is already running")
		return 0
	}
	defer unlock()
	return runRecoverScan(env, o)
}

func parseRecoverFlags(env Env, args []string) (recoverOpts, error) {
	fs := flag.NewFlagSet("recover", flag.ContinueOnError)
	fs.SetOutput(env.Stderr)
	o := recoverOpts{maxAge: defaultMaxAge}
	fs.BoolVar(&o.dryRun, "dry-run", false, "list the runs recover would resume")
	fs.DurationVar(&o.maxAge, "max-age", defaultMaxAge, "skip runs older than this")
	if err := fs.Parse(args); err != nil {
		return o, err
	}
	if fs.NArg() != 0 {
		return o, usagef("unexpected arguments")
	}
	return o, nil
}

func runRecoverScan(env Env, o recoverOpts) int {
	ids, err := candidateRunIDs(env)
	if err != nil {
		fmt.Fprintf(env.Stderr, "error: %v\n", err)
		return 1
	}
	cutoff := env.Host.Now().Add(-o.maxAge)
	spawned := 0
	noted := false
	for _, id := range ids {
		note, spawn := recoverOne(env, id, cutoff, o)
		spawned += spawn
		if note != "" {
			noted = true
		}
	}
	if !o.dryRun && spawned == 0 && !noted {
		fmt.Fprintln(env.Stderr, "nothing to recover")
	}
	return 0
}

func recoverOne(env Env, id string, cutoff time.Time, o recoverOpts) (string, int) {
	recs, st, err := replayRunRecords(env, id)
	if err != nil {
		return skipNote(env, id, err), 0
	}
	if skip, why := notRecoverable(env, id, recs, st, cutoff, o.maxAge); skip {
		return skipNote(env, id, why), 0
	}
	prev := loadRecoverState(env, id)
	attempts := prev.Attempts
	if prev.Records != workedRecords(recs) {
		attempts = 0
	}
	if attempts >= crashLimit {
		return skipNote(env, id, fmt.Errorf("keeps crashing after %d recoveries", crashLimit)), 0
	}
	if o.dryRun {
		fmt.Fprintf(env.Stdout, "would recover %s\n", id)
		return "", 0
	}
	if err := saveRecoverState(env, id, recoverState{Records: workedRecords(recs), Attempts: attempts + 1}); err != nil {
		fmt.Fprintf(env.Stderr, "error: %v\n", err)
		return "", 0
	}
	if recoverSpawn(env, id) == 0 {
		return "", 0
	}
	fmt.Fprintf(env.Stdout, "recovered %s\n", id)
	return "recovered", 1
}

func skipNote(env Env, id string, why error) string {
	if why != nil {
		fmt.Fprintf(env.Stderr, "skipping %s: %v\n", id, why)
	}
	return "skipped"
}

func recoverSpawn(env Env, id string) int {
	pid, err := env.Spawn([]string{"resume", id}, runLogPath(env.Getenv, id))
	if err != nil {
		fmt.Fprintf(env.Stderr, "error: %v\n", err)
		return 0
	}
	if err := writeRunPidFor(env, id, pid); err != nil {
		fmt.Fprintf(env.Stderr, "error: %v\n", err)
		return 0
	}
	return 1
}

func notRecoverable(env Env, id string, recs []journal.Record, st journal.State, cutoff time.Time, maxAge time.Duration) (skip bool, why error) {
	if !st.Start.Detached && !st.Detached {
		return true, nil
	}
	if st.Finished {
		return true, nil
	}
	if st.Stopped {
		return true, nil
	}
	if runAlive(env, id) {
		return true, nil
	}
	if !recs[0].TS.IsZero() && recs[0].TS.Before(cutoff) {
		return true, fmt.Errorf("older than %s", maxAge)
	}
	if _, _, err := resolveResumeWiring(env, st); err != nil {
		return true, err
	}
	return false, nil
}

func resolveResumeWiring(env Env, st journal.State) (Endpoint, map[string]Endpoint, error) {
	primary, bySpec, err := resolveResumeSpecs(st.Start, env.Getenv)
	if err != nil {
		return Endpoint{}, nil, err
	}
	if st.Current == "" {
		return primary, bySpec, nil
	}
	if _, ok := bySpec[st.Current]; ok {
		return primary, bySpec, nil
	}
	ep, err := endpointFor(primary, st.Current, env.Getenv)
	if err != nil {
		return primary, bySpec, err
	}
	bySpec[st.Current] = ep
	return primary, bySpec, nil
}

func noteSkipped(env Env, id string, err error) {
	fmt.Fprintf(env.Stderr, "skipping %s: %v\n", id, err)
}

func candidateRunIDs(env Env) ([]string, error) {
	dir := runsDir(env.Getenv)
	ents, err := env.Host.ReadDir(dir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	return runIDs(ents), nil
}

func replayRunRecords(env Env, id string) ([]journal.Record, journal.State, error) {
	recs, err := journal.Read(env.Host, runPath(env.Getenv, id))
	if err != nil {
		return nil, journal.State{}, err
	}
	st, err := journal.Replay(recs)
	if err != nil {
		return recs, journal.State{}, err
	}
	return recs, st, nil
}

func runPath(getenv func(string) string, id string) string {
	return filepath.Join(runsDir(getenv), id+".jsonl")
}

func workedRecords(recs []journal.Record) int {
	n := 0
	for _, rec := range recs {
		if rec.Type == journal.TypeDetached {
			continue
		}
		n++
	}
	return n
}

type recoverState struct {
	Records  int `json:"records"`
	Attempts int `json:"attempts"`
}

func loadRecoverState(env Env, id string) recoverState {
	var st recoverState
	data, err := env.Host.ReadFile(recoverStatePath(env.Getenv, id))
	if err != nil {
		return st
	}
	_ = json.Unmarshal(data, &st)
	return st
}

func saveRecoverState(env Env, id string, st recoverState) error {
	data, err := json.Marshal(st)
	if err != nil {
		return err
	}
	return env.Host.WriteFile(recoverStatePath(env.Getenv, id), data)
}

func recoverStatePath(getenv func(string) string, id string) string {
	return filepath.Join(runsDir(getenv), id+".recover")
}

func recoverLockPath(getenv func(string) string) string {
	return filepath.Join(runsDir(getenv), "recover.lock")
}
