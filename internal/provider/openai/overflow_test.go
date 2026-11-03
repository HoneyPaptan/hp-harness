package openai

import (
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/HoneyPaptan/hp-harness/internal/provider"
)

func TestErrorFromResponseOverflow(t *testing.T) {
	overflowBody := `{"error":{"code":"context_length_exceeded","message":"too long"}}`
	resp := &http.Response{
		StatusCode: 400,
		Body:       io.NopCloser(strings.NewReader(overflowBody)),
	}
	err := errorFromResponse(resp)
	var he *provider.HTTPError
	if !errors.As(err, &he) || !errors.Is(err, provider.ErrContextOverflow) {
		t.Fatalf("err = %#v", err)
	}

	groqBody := `{"error":{"message":"Please reduce the length of the messages or completion."}}`
	resp.Body = io.NopCloser(strings.NewReader(groqBody))
	err = errorFromResponse(resp)
	if !errors.As(err, &he) || !errors.Is(err, provider.ErrContextOverflow) {
		t.Fatalf("groq err = %#v", err)
	}

	normalBody := `{"error":{"message":"bad request","type":"invalid_request_error"}}`
	resp.Body = io.NopCloser(strings.NewReader(normalBody))
	err = errorFromResponse(resp)
	if !errors.As(err, &he) || errors.Is(err, provider.ErrContextOverflow) {
		t.Fatalf("normal err = %#v", err)
	}
}

func TestStreamErrorOverflow(t *testing.T) {
	frame := `{"error":{"code":"context_length_exceeded","message":"too long"}}`
	err := readAll(t.Context(), strings.NewReader(sseBody(chunkText("par"), frame)))
	if !errors.Is(err, provider.ErrContextOverflow) {
		t.Fatalf("err = %#v", err)
	}
}

func TestErrorFromResponseOverflowTransientWrap(t *testing.T) {
	body := `{"error":{"code":"context_length_exceeded","message":"too long"}}`
	resp := &http.Response{
		StatusCode: 500,
		Body:       io.NopCloser(strings.NewReader(body)),
	}
	err := errorFromResponse(resp)
	var te *provider.TransientError
	if !errors.As(err, &te) || !errors.Is(err, provider.ErrContextOverflow) {
		t.Fatalf("err = %#v", err)
	}
}
