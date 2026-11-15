package cli

import (
	"context"
	"net/http"

	"github.com/HoneyPaptan/hp-harness/internal/provider"
	"github.com/HoneyPaptan/hp-harness/internal/web"
)

const EmbedHost = "hp.local"

type WebEmbed struct {
	Handler http.Handler
	Token   string
	Close   func()
}

func NewWebEmbed(env Env, args []string) (*WebEmbed, error) {
	o, err := parseWebFlags(env, args)
	if err != nil {
		return nil, err
	}
	if err := checkWebStart(env, o.run); err != nil {
		return nil, err
	}
	dir, err := resolveWorkDir(env, o.run.dir)
	if err != nil {
		return nil, err
	}
	token, err := web.NewToken(env.Rand)
	if err != nil {
		return nil, err
	}
	pool := newToolPool(webIdleTimeout)
	srv, err := web.New(web.Options{
		Backend: &webBackend{env: env, opts: o.run, dir: dir, pool: pool}, Token: token,
		Hosts: []string{EmbedHost},
	})
	if err != nil {
		pool.close()
		return nil, err
	}
	return &WebEmbed{Handler: srv.Handler(), Token: token, Close: func() { srv.Close(); pool.close() }}, nil
}

func (b *webBackend) Models(ctx context.Context, preset string) ([]string, error) {
	ep, err := presetEndpoint(Endpoint{}, preset, b.env.Getenv)
	if err != nil {
		return nil, invalid(err)
	}
	listCtx, cancel := context.WithTimeout(ctx, modelListTimeout)
	defer cancel()
	return provider.ListModels(listCtx, b.env.NewProvider(ep))
}
