package cli

import (
	"sync"
)

var (
	execEnvMu   sync.Mutex
	execEnvFn   func() []string
	execEnvInit func(Env, string) func() []string
)

func SetExecEnvInit(init func(Env, string) func() []string) {
	execEnvInit = init
}

func ResetExecEnvForTest(init func(Env, string) func() []string) {
	execEnvMu.Lock()
	execEnvFn = nil
	execEnvInit = init
	execEnvMu.Unlock()
}

func sharedExecEnv(env Env, dir string) func() []string {
	execEnvMu.Lock()
	defer execEnvMu.Unlock()
	if execEnvFn == nil && execEnvInit != nil {
		execEnvFn = execEnvInit(env, dir)
	}
	return execEnvFn
}

func kickExecEnv(env Env, dir string) {
	sharedExecEnv(env, dir)
}

func SharedExecEnvForTest(env Env, dir string) func() []string {
	return sharedExecEnv(env, dir)
}
