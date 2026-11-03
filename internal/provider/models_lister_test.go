package provider

import (
	"context"
	"errors"
	"iter"
	"testing"
)

type plainProvider struct{}

func (plainProvider) Stream(context.Context, Request) iter.Seq2[Event, error] {
	return func(yield func(Event, error) bool) {}
}

type stubLister struct {
	ids []string
}

func (s stubLister) Stream(context.Context, Request) iter.Seq2[Event, error] {
	return func(yield func(Event, error) bool) {}
}

func (s stubLister) ListModels(context.Context) ([]string, error) {
	return s.ids, nil
}

func TestListModelsWithLister(t *testing.T) {
	want := []string{"a", "b"}
	got, err := ListModels(context.Background(), stubLister{ids: want})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestListModelsWithoutLister(t *testing.T) {
	_, err := ListModels(context.Background(), plainProvider{})
	if !errors.Is(err, ErrNoModelList) {
		t.Fatalf("err = %v", err)
	}
}
