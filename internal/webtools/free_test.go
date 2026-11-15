package webtools

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/HoneyPaptan/hp-harness/internal/tool"
)

func freeServer(t *testing.T) (*httptest.Server, *[]string) {
	t.Helper()
	var agents []string
	mux := http.NewServeMux()
	mux.HandleFunc("/ddg/", func(w http.ResponseWriter, r *http.Request) {
		agents = append(agents, r.Header.Get("User-Agent"))
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(`{"Heading":"Go (programming language)","AbstractText":"Go is a language.","AbstractURL":"` + articleURL("http://"+r.Host, "Go (programming language)") + `","AbstractSource":"Wikipedia","Answer":""}`))
	})
	mux.HandleFunc("/w/api.php", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("origin") != "*" {
			t.Errorf("wikipedia call without origin=*: %s", r.URL)
		}
		if r.URL.Query().Get("list") == "search" {
			_, _ = w.Write([]byte(`{"query":{"search":[{"title":"Go (programming language)","snippet":"x"},{"title":"Gopher","snippet":"The <span class=\"searchmatch\">gopher</span> &amp; friends"}]}}`))
			return
		}
		if r.URL.Query().Get("titles") == "Nowhere" {
			_, _ = w.Write([]byte(`{"query":{"pages":{"-1":{"title":"Nowhere","missing":""}}}}`))
			return
		}
		_, _ = w.Write([]byte(`{"query":{"pages":{"1":{"title":"Gopher","extract":"Gophers dig."}}}}`))
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, &agents
}

func freeCfg(srv *httptest.Server) FreeConfig {
	return FreeConfig{Client: srv.Client(), Wikipedia: srv.URL, DuckDuckGo: srv.URL + "/ddg/", UserAgent: "hp-test"}
}

func TestFreeSearchMergesAndDedupes(t *testing.T) {
	srv, agents := freeServer(t)
	cfg := freeCfg(srv)
	res, err := FreeSearchTool(cfg).Run(context.Background(), tool.Env{}, json.RawMessage(`{"query":"golang"}`))
	if err != nil || res.IsError {
		t.Fatalf("search failed: %v %+v", err, res)
	}
	if strings.Count(res.Text, "Go (programming language)") != 1 {
		t.Fatalf("expected the instant answer first and no duplicate:\n%s", res.Text)
	}
	if !strings.Contains(res.Text, "Gopher (Wikipedia)") || !strings.Contains(res.Text, "The gopher & friends") {
		t.Fatalf("expected plain wikipedia snippet:\n%s", res.Text)
	}
	if !strings.HasPrefix(res.Text, "1. Go (programming language) (Wikipedia)") {
		t.Fatalf("instant answer should lead:\n%s", res.Text)
	}
	if len(*agents) == 0 || (*agents)[0] != "hp-test" {
		t.Fatalf("user agent not sent: %v", *agents)
	}
}

func TestWikiReadByTitleAndURL(t *testing.T) {
	srv, _ := freeServer(t)
	cfg := freeCfg(srv)
	for _, page := range []string{"Gopher", srv.URL + "/wiki/Gopher"} {
		res, err := WikiReadTool(cfg).Run(context.Background(), tool.Env{}, json.RawMessage(`{"page":"`+page+`"}`))
		if err != nil {
			t.Fatal(err)
		}
		if page != "Gopher" {
			if !res.IsError {
				t.Fatalf("plain http URL must be refused: %s", res.Text)
			}
			continue
		}
		if res.IsError || !strings.Contains(res.Text, "Gophers dig.") {
			t.Fatalf("unexpected read: %+v", res)
		}
	}
}

func TestWikiReadRefusesOtherHostsAndMissing(t *testing.T) {
	srv, _ := freeServer(t)
	cfg := freeCfg(srv)
	for _, page := range []string{"https://example.com/wiki/Go", "Nowhere"} {
		res, _ := WikiReadTool(cfg).Run(context.Background(), tool.Env{}, json.RawMessage(`{"page":"`+page+`"}`))
		if !res.IsError {
			t.Fatalf("%s should fail, got %s", page, res.Text)
		}
	}
}
