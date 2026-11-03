package cli

import (
	"net/http"
	"time"

	"github.com/HoneyPaptan/hp-harness/internal/jev"
	"github.com/HoneyPaptan/hp-harness/internal/tool"
	"github.com/HoneyPaptan/hp-harness/internal/webtools"
)

func toolExtras(env Env, client *jev.Client, st settings) []tool.Tool {
	return append(jevTools(client), webTools(env, st)...)
}

func webTools(env Env, st settings) []tool.Tool {
	if st.Web != nil && !*st.Web {
		return nil
	}
	allowPrivate := env.Getenv("HP_WEB_ALLOW_PRIVATE") == "1"
	httpClient := &http.Client{Timeout: 20 * time.Second}
	out := []tool.Tool{webtools.FetchTool(webtools.NewClient(allowPrivate))}
	if key := env.Getenv("EXA_API_KEY"); key != "" {
		out = append(out, webtools.SearchTool(httpClient, webtools.ExaSearchURL, key))
	}
	return out
}
