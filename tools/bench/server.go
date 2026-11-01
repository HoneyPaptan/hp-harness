package main

import (
	"io"
	"net"
	"net/http"
)

const fakeReply = `data: {"choices":[{"delta":{"content":"hi"}}]}

data: {"choices":[{"delta":{},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1}}

data: [DONE]

`

type fakeServer struct {
	URL string
	srv *http.Server
}

func (s *fakeServer) Close() {
	s.srv.Close()
}

func startFakeServer() (*fakeServer, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	srv := &http.Server{Handler: http.HandlerFunc(replyInstantly)}
	go srv.Serve(ln)
	return &fakeServer{URL: "http://" + ln.Addr().String() + "/v1", srv: srv}, nil
}

func replyInstantly(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	io.WriteString(w, fakeReply)
}
