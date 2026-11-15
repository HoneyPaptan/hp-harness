//go:build js && wasm

package main

import (
	"context"
	"errors"
	"io/fs"
	"net/http"
	"strings"
	"syscall/js"

	"github.com/HoneyPaptan/hp-harness/internal/cli"
)

func (a *app) request(_ js.Value, args []js.Value) any {
	if a.embed == nil {
		return jsError(errors.New("hp is not booted"))
	}
	if len(args) < 2 {
		return jsError(errors.New("request needs a request object and a chunk callback"))
	}
	in, onChunk := args[0], args[1]
	ctx, cancel := context.WithCancel(context.Background())
	req, err := a.newRequest(ctx, in)
	if err != nil {
		cancel()
		return jsError(err)
	}
	out := js.Global().Get("Object").New()
	out.Set("abort", js.FuncOf(func(js.Value, []js.Value) any { cancel(); return nil }))
	out.Set("done", newPromise(func(resolve, reject js.Value) {
		go func() {
			defer cancel()
			w := &chunkWriter{header: http.Header{}, onChunk: onChunk}
			a.embed.Handler.ServeHTTP(w, req)
			resolve.Invoke(w.statusCode())
		}()
	}))
	return out
}

func (a *app) newRequest(ctx context.Context, in js.Value) (*http.Request, error) {
	method, path := in.Get("method").String(), in.Get("path").String()
	var body string
	if b := in.Get("body"); b.Type() == js.TypeString {
		body = b.String()
	}
	req, err := http.NewRequestWithContext(ctx, method, "http://"+cli.EmbedHost+path, strings.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Host = cli.EmbedHost
	req.Header.Set("Authorization", "Bearer "+a.embed.Token)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if method != http.MethodGet {
		req.Header.Set("Origin", "http://"+cli.EmbedHost)
	}
	return req, nil
}

type chunkWriter struct {
	header  http.Header
	status  int
	onChunk js.Value
}

func (w *chunkWriter) Header() http.Header { return w.header }

func (w *chunkWriter) WriteHeader(code int) {
	if w.status == 0 {
		w.status = code
	}
}

func (w *chunkWriter) Write(p []byte) (int, error) {
	w.WriteHeader(http.StatusOK)
	w.onChunk.Invoke(string(p))
	return len(p), nil
}

func (w *chunkWriter) Flush() {}

func (w *chunkWriter) statusCode() int {
	if w.status == 0 {
		return http.StatusOK
	}
	return w.status
}

func (a *app) readFile(_ js.Value, args []js.Value) any {
	if len(args) < 1 {
		return nil
	}
	data, err := a.fs.ReadFile(args[0].String())
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return jsError(err)
	}
	return bytesToJS(data)
}

func (a *app) writeFile(_ js.Value, args []js.Value) any {
	if len(args) < 2 {
		return jsError(errors.New("writeFile needs a path and bytes"))
	}
	if err := a.fs.WriteFile(args[0].String(), bytesFromJS(args[1])); err != nil {
		return jsError(err)
	}
	return nil
}

func (a *app) removeFile(_ js.Value, args []js.Value) any {
	if len(args) < 1 {
		return nil
	}
	if err := a.fs.Remove(args[0].String()); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return jsError(err)
	}
	return nil
}

func newPromise(run func(resolve, reject js.Value)) js.Value {
	var executor js.Func
	executor = js.FuncOf(func(_ js.Value, args []js.Value) any {
		executor.Release()
		run(args[0], args[1])
		return nil
	})
	return js.Global().Get("Promise").New(executor)
}

func jsError(err error) js.Value {
	return js.Global().Get("Error").New(err.Error())
}

func bytesToJS(b []byte) js.Value {
	arr := js.Global().Get("Uint8Array").New(len(b))
	js.CopyBytesToJS(arr, b)
	return arr
}

func bytesFromJS(v js.Value) []byte {
	if v.Type() == js.TypeString {
		return []byte(v.String())
	}
	b := make([]byte, v.Get("length").Int())
	js.CopyBytesToGo(b, v)
	return b
}

func keys(obj js.Value) []string {
	if obj.IsUndefined() || obj.IsNull() {
		return nil
	}
	return stringsOf(js.Global().Get("Object").Call("keys", obj))
}

func stringsOf(arr js.Value) []string {
	if arr.IsUndefined() || arr.IsNull() {
		return nil
	}
	out := make([]string, arr.Length())
	for i := range out {
		out[i] = arr.Index(i).String()
	}
	return out
}
