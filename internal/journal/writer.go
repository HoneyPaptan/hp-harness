package journal

import (
	"encoding/json"
	"errors"
	"io/fs"

	"github.com/HoneyPaptan/hp-harness/internal/host"
	"github.com/HoneyPaptan/hp-harness/internal/provider"
)

type Writer struct {
	h    host.Host
	path string
	seq  int
}

func Create(h host.Host, path string, start RunStart) (*Writer, error) {
	if err := ensureAbsent(h, path); err != nil {
		return nil, err
	}
	w := &Writer{h: h, path: path, seq: 0}
	if err := w.append(TypeRunStart, start); err != nil {
		return nil, err
	}
	return w, nil
}

func ensureAbsent(h host.Host, path string) error {
	_, err := h.Stat(path)
	if err == nil {
		return &fs.PathError{Op: "create", Path: path, Err: fs.ErrExist}
	}
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	return err
}

func (w *Writer) User(text string) error {
	return w.append(TypeUser, User{Text: text})
}

func (w *Writer) Assistant(m provider.Message) error {
	return w.append(TypeAssistant, Assistant{Message: m})
}

func (w *Writer) ToolStarted(c provider.ToolCall) error {
	return w.append(TypeToolStarted, ToolStarted{Call: c})
}

func (w *Writer) ToolResult(r provider.ToolResult) error {
	return w.append(TypeToolResult, ToolResultData{Result: r})
}

func (w *Writer) End(reason string) error {
	return w.append(TypeRunEnd, RunEnd{Reason: reason})
}

func (w *Writer) append(t Type, payload any) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	rec := Record{
		Seq:  w.seq,
		TS:   w.h.Now(),
		Type: t,
		Data: data,
	}
	line, err := json.Marshal(rec)
	if err != nil {
		return err
	}
	line = append(line, '\n')
	if err := w.h.Append(w.path, line); err != nil {
		return err
	}
	w.seq++
	return nil
}
