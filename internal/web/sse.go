package web

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"
)

const writeWait = 10 * time.Second

type stream struct {
	w  http.ResponseWriter
	rc *http.ResponseController
}

func newStream(w http.ResponseWriter) *stream {
	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-store")
	h.Set("X-Accel-Buffering", "no")
	return &stream{w: w, rc: http.NewResponseController(w)}
}

func (st *stream) write(frame []byte) error {
	_ = st.rc.SetWriteDeadline(time.Now().Add(writeWait))
	if _, err := st.w.Write(frame); err != nil {
		return err
	}
	return st.rc.Flush()
}

func (st *stream) send(ev Event) error {
	data, err := json.Marshal(ev)
	if err != nil {
		return err
	}
	return st.write(fmt.Appendf(nil, "id: %d\ndata: %s\n\n", ev.ID, data))
}

func (st *stream) comment(text string) error {
	return st.write([]byte(": " + text + "\n\n"))
}

func lastEventID(r *http.Request) (int64, bool) {
	raw := r.Header.Get("Last-Event-ID")
	if raw == "" {
		return 0, true
	}
	id, err := strconv.ParseInt(raw, 10, 64)
	return id, err == nil && id >= 0
}

func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	after, ok := lastEventID(r)
	if !ok {
		writeError(w, http.StatusBadRequest, "bad Last-Event-ID")
		return
	}
	id, ok := runID(w, r)
	if !ok {
		return
	}
	rn, err := s.mgr.run(id)
	if err != nil {
		writeFailure(w, err)
		return
	}
	rn.refresh()
	backlog, sub := rn.subscribe(after)
	defer rn.unsubscribe(sub)
	s.follow(r, newStream(w), rn, backlog, sub)
}

func (s *Server) follow(r *http.Request, st *stream, rn *run, backlog []Event, sub *subscriber) {
	if st.comment("connected") != nil {
		return
	}
	for _, ev := range backlog {
		if st.send(ev) != nil {
			return
		}
	}
	beat := time.NewTicker(s.heartbeat)
	defer beat.Stop()
	for {
		select {
		case ev := <-sub.ch:
			if st.send(ev) != nil {
				return
			}
		case <-beat.C:
			if st.comment("heartbeat") != nil {
				return
			}
			rn.refresh()
		case <-sub.dropped:
			return
		case <-r.Context().Done():
			return
		case <-s.closing:
			return
		}
	}
}
