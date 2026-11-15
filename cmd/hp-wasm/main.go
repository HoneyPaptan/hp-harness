//go:build js && wasm

package main

import (
	"crypto/rand"
	"errors"
	"math/big"
	"sync"
	"syscall/js"

	"github.com/HoneyPaptan/hp-harness/internal/cli"
	"github.com/HoneyPaptan/hp-harness/internal/host"
	"github.com/HoneyPaptan/hp-harness/internal/provider"
	"github.com/HoneyPaptan/hp-harness/internal/provider/openai"
	"github.com/HoneyPaptan/hp-harness/internal/provider/transport"
)

var version = "dev"

type vars struct {
	mu sync.RWMutex
	m  map[string]string
}

func (v *vars) get(k string) string {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return v.m[k]
}

func (v *vars) set(k, val string) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if val == "" {
		delete(v.m, k)
		return
	}
	v.m[k] = val
}

type app struct {
	env   *vars
	fs    *host.Browser
	embed *cli.WebEmbed
}

func main() {
	a := &app{env: &vars{m: map[string]string{}}, fs: host.NewBrowser()}
	api := js.Global().Get("Object").New()
	api.Set("version", version)
	api.Set("boot", js.FuncOf(a.boot))
	api.Set("setEnv", js.FuncOf(a.setEnv))
	api.Set("request", js.FuncOf(a.request))
	api.Set("readFile", js.FuncOf(a.readFile))
	api.Set("writeFile", js.FuncOf(a.writeFile))
	api.Set("removeFile", js.FuncOf(a.removeFile))
	js.Global().Set("hp", api)
	if ready := js.Global().Get("hpReady"); ready.Type() == js.TypeFunction {
		ready.Invoke(api)
	}
	select {}
}

func (a *app) setEnv(_ js.Value, args []js.Value) any {
	if len(args) == 2 {
		a.env.set(args[0].String(), args[1].String())
	}
	return nil
}

func (a *app) boot(_ js.Value, args []js.Value) any {
	if a.embed != nil {
		return a.embed.Token
	}
	if len(args) < 1 {
		return jsError(errors.New("boot needs a config"))
	}
	cfg := args[0]
	for _, k := range keys(cfg.Get("env")) {
		a.env.set(k, cfg.Get("env").Get(k).String())
	}
	if err := a.restore(cfg.Get("files")); err != nil {
		return jsError(err)
	}
	cwd := cfg.Get("cwd").String()
	a.fs.Mkdir(cwd)
	if on := cfg.Get("onChange"); on.Type() == js.TypeFunction {
		a.fs.OnChange(func(c host.Change) { on.Invoke(string(c.Op), c.Path, bytesToJS(c.Data)) })
	}
	embed, err := cli.NewWebEmbed(a.newEnv(cwd), stringsOf(cfg.Get("args")))
	if err != nil {
		return jsError(err)
	}
	a.embed = embed
	return embed.Token
}

func (a *app) restore(files js.Value) error {
	if files.IsUndefined() || files.IsNull() {
		return nil
	}
	for i := 0; i < files.Length(); i++ {
		f := files.Index(i)
		if err := a.fs.Restore(f.Index(0).String(), bytesFromJS(f.Index(1))); err != nil {
			return err
		}
	}
	return nil
}

func (a *app) newEnv(cwd string) cli.Env {
	pid := sessionPid()
	return cli.Env{
		Getenv: a.env.get, Cwd: cwd, Stdout: console{"log"}, Stderr: console{"warn"},
		Host: a.fs, Rand: rand.Reader, Version: version, NewProvider: cli.SharedProviders(newProvider),
		Alive: func(p int) bool { return p == pid }, Pid: pid, Lock: a.fs.Lock, Os: "js", Browser: true,
	}
}

func sessionPid() int {
	n, err := rand.Int(rand.Reader, big.NewInt(1<<30))
	if err != nil {
		return 1
	}
	return int(n.Int64()) + 2
}

func newProvider(ep cli.Endpoint) provider.Provider {
	return &openai.Client{
		BaseURL: ep.BaseURL, APIKey: ep.APIKey, HTTP: transport.NewHTTPClient(),
		DisableThinking: ep.Preset == "deepseek", NoStreamOptions: ep.Preset == "mistral",
	}
}

type console struct{ level string }

func (c console) Write(p []byte) (int, error) {
	js.Global().Get("console").Call(c.level, "[hp] "+string(p))
	return len(p), nil
}
