package anthropic

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strings"

	"github.com/HoneyPaptan/hp-harness/internal/provider/transport"
)

const (
	maxModelsListBody = 4 << 20
	modelsPageLimit   = "1000"
	maxModelsPages    = 10
)

type wireModelsList struct {
	Data    []wireModelEntry `json:"data"`
	HasMore bool             `json:"has_more"`
	LastID  string           `json:"last_id"`
}

type wireModelEntry struct {
	ID string `json:"id"`
}

func (c *Client) ListModels(ctx context.Context) ([]string, error) {
	var entries []wireModelEntry
	afterID := ""
	for page := 0; page < maxModelsPages; page++ {
		pageList, err := c.fetchModelsPage(ctx, afterID)
		if err != nil {
			return nil, err
		}
		entries = append(entries, pageList.Data...)
		if !pageList.HasMore || pageList.LastID == "" {
			break
		}
		afterID = pageList.LastID
	}
	return normalizeModelIDs(entries), nil
}

func (c *Client) fetchModelsPage(ctx context.Context, afterID string) (wireModelsList, error) {
	httpReq, err := c.newModelsListRequest(ctx, afterID)
	if err != nil {
		return wireModelsList{}, err
	}
	resp, err := c.httpClient().Do(httpReq)
	if err != nil {
		return wireModelsList{}, transport.Failure(ctx, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return wireModelsList{}, errorFromResponse(resp)
	}
	return decodeModelsPage(resp.Body)
}

func (c *Client) newModelsListRequest(ctx context.Context, afterID string) (*http.Request, error) {
	rawURL := strings.TrimSuffix(c.BaseURL, "/") + "/models"
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, err
	}
	q := u.Query()
	q.Set("limit", modelsPageLimit)
	if afterID != "" {
		q.Set("after_id", afterID)
	}
	u.RawQuery = q.Encode()
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	c.setAnthropicHeaders(httpReq.Header)
	return httpReq, nil
}

func decodeModelsPage(r io.Reader) (wireModelsList, error) {
	body, err := io.ReadAll(io.LimitReader(r, maxModelsListBody))
	if err != nil {
		return wireModelsList{}, err
	}
	var list wireModelsList
	if err := json.Unmarshal(body, &list); err != nil {
		return wireModelsList{}, err
	}
	return list, nil
}

func normalizeModelIDs(entries []wireModelEntry) []string {
	seen := make(map[string]struct{})
	var ids []string
	for _, e := range entries {
		id := strings.TrimSpace(e.ID)
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
