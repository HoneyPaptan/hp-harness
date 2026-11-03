package openai

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"slices"
	"strings"

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
	return decodeModelIDs(resp.Body, true)
}

func decodeModelIDs(r io.Reader, stripModelsPrefix bool) ([]string, error) {
	body, err := io.ReadAll(io.LimitReader(r, maxModelsListBody))
	if err != nil {
		return nil, err
	}
	var list wireModelsList
	if err := json.Unmarshal(body, &list); err != nil {
		return nil, err
	}
	return normalizeModelIDs(list.Data, stripModelsPrefix), nil
}

func normalizeModelIDs(entries []wireModelEntry, stripModelsPrefix bool) []string {
	seen := make(map[string]struct{})
	var ids []string
	for _, e := range entries {
		id := strings.TrimSpace(e.ID)
		if id == "" {
			continue
		}
		if stripModelsPrefix {
			id = strings.TrimPrefix(id, "models/")
		}
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}
	slices.Sort(ids)
	return ids
}
