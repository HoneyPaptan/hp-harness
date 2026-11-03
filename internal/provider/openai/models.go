package openai

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/HoneyPaptan/hp-harness/internal/provider"
	"github.com/HoneyPaptan/hp-harness/internal/provider/transport"
)

const maxModelsListBody = 4 << 20

type wireModelsList struct {
	Data []wireModelEntry `json:"data"`
}

type wireModelEntry struct {
	ID string `json:"id"`
}

func (c *Client) ListModels(ctx context.Context) ([]string, error) {
	url := strings.TrimSuffix(c.BaseURL, "/") + "/models"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	c.setAuthorization(httpReq.Header)
	resp, err := c.httpClient().Do(httpReq)
	if err != nil {
		return nil, transport.Failure(ctx, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, errorFromResponse(resp)
	}
	return decodeModelIDs(resp.Body)
}

func decodeModelIDs(r io.Reader) ([]string, error) {
	body, err := io.ReadAll(io.LimitReader(r, maxModelsListBody))
	if err != nil {
		return nil, err
	}
	var list wireModelsList
	if err := json.Unmarshal(body, &list); err != nil {
		return nil, err
	}
	raw := make([]string, 0, len(list.Data))
	for _, e := range list.Data {
		raw = append(raw, strings.TrimPrefix(strings.TrimSpace(e.ID), "models/"))
	}
	return provider.CleanModelIDs(raw), nil
}
