package transport

import (
	"context"
	"net/http"
	"time"
)

const (
	idleConnsPerHost = 4
	preconnectWait   = 5 * time.Second
)

func NewHTTPClient() *http.Client {
	t := http.DefaultTransport.(*http.Transport).Clone()
	t.MaxIdleConnsPerHost = idleConnsPerHost
	return &http.Client{Transport: t}
}

func Preconnect(ctx context.Context, client *http.Client, baseURL string) {
	ctx, cancel := context.WithTimeout(ctx, preconnectWait)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, baseURL, nil)
	if err != nil {
		return
	}
	probe := *client
	probe.CheckRedirect = keepFirstResponse
	resp, err := probe.Do(req)
	if err == nil {
		resp.Body.Close()
	}
}

func keepFirstResponse(*http.Request, []*http.Request) error {
	return http.ErrUseLastResponse
}
