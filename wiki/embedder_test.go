package wiki

import (
	"context"
	"errors"
	"testing"

	"github.com/tingly-dev/tingly-wiki/llm"
)

func TestNoopEmbedder(t *testing.T) {
	e := NoopEmbedder{}
	vec, err := e.Embed(context.Background(), "anything")
	if err != nil {
		t.Errorf("NoopEmbedder.Embed should not error, got: %v", err)
	}
	if vec != nil {
		t.Errorf("NoopEmbedder.Embed should return nil, got: %v", vec)
	}
	if d := e.Dim(); d != 0 {
		t.Errorf("NoopEmbedder.Dim() = %d, want 0", d)
	}
}

func TestLLMEmbedder_DimensionCaching(t *testing.T) {
	mock := &llm.MockLLM{
		EmbedFunc: func(ctx context.Context, text string) ([]float32, error) {
			return make([]float32, 128), nil
		},
	}
	e := NewLLMEmbedder(mock)

	// Before any call, Dim should be 0
	if d := e.Dim(); d != 0 {
		t.Errorf("Dim before first Embed = %d, want 0", d)
	}

	// First call caches the dimension
	vec, err := e.Embed(context.Background(), "hello")
	if err != nil {
		t.Fatalf("Embed failed: %v", err)
	}
	if len(vec) != 128 {
		t.Errorf("vec length = %d, want 128", len(vec))
	}
	if d := e.Dim(); d != 128 {
		t.Errorf("Dim after first Embed = %d, want 128", d)
	}

	// Subsequent calls should not change Dim even if vector size differs
	mock.EmbedFunc = func(ctx context.Context, text string) ([]float32, error) {
		return make([]float32, 256), nil
	}
	_, _ = e.Embed(context.Background(), "second call")
	if d := e.Dim(); d != 128 {
		t.Errorf("Dim should remain cached at 128 after second call, got %d", d)
	}
}

func TestLLMEmbedder_ErrorPassthrough(t *testing.T) {
	wantErr := errors.New("embedding-not-supported")
	mock := &llm.MockLLM{
		EmbedFunc: func(ctx context.Context, text string) ([]float32, error) {
			return nil, wantErr
		},
	}
	e := NewLLMEmbedder(mock)
	_, err := e.Embed(context.Background(), "x")
	if !errors.Is(err, wantErr) {
		t.Errorf("error = %v, want %v", err, wantErr)
	}
}
