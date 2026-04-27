package llm

import (
	"context"
	"os"
	"testing"

	"github.com/tingly-dev/tingly-wiki/schema"
)

// TestOpenAIAdapterWithRealAPI tests the OpenAI adapter with a real API call
// This test requires OPENAI_API_KEY to be set
func TestOpenAIAdapterWithRealAPI(t *testing.T) {
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		t.Skip("OPENAI_API_KEY not set, skipping OpenAI adapter test")
	}

	adapter, err := NewOpenAIAdapter(&OpenAIConfig{
		APIKey: apiKey,
		Model:  "gpt-4o-mini",
	})
	if err != nil {
		t.Fatalf("Failed to create OpenAI adapter: %v", err)
	}

	t.Run("Summarize", func(t *testing.T) {
		summary, err := adapter.Summarize(context.Background(), "OpenAI is an AI research organization that develops GPT models.")
		if err != nil {
			t.Fatalf("Failed to summarize: %v", err)
		}
		if summary == "" {
			t.Error("Summary is empty")
		}
		t.Logf("Summary: %s", summary)
	})

	t.Run("Extract", func(t *testing.T) {
		info, err := adapter.Extract(context.Background(),
			"OpenAI develops ChatGPT, which is an AI assistant. GPT-4 is their latest model.",
			schema.DefaultSchema())
		if err != nil {
			t.Fatalf("Failed to extract: %v", err)
		}
		if info.Summary == "" {
			t.Error("Summary is empty")
		}
		if len(info.Entities) == 0 {
			t.Error("No entities extracted")
		}
		t.Logf("Extracted: %+v", info)
	})
}

// TestAnthropicAdapterWithRealAPI tests the Anthropic adapter with a real API call
// This test requires ANTHROPIC_API_KEY to be set
func TestAnthropicAdapterWithRealAPI(t *testing.T) {
	apiKey := os.Getenv("ANTHROPIC_API_KEY")
	if apiKey == "" {
		t.Skip("ANTHROPIC_API_KEY not set, skipping Anthropic adapter test")
	}

	adapter, err := NewAnthropicAdapter(&AnthropicConfig{
		APIKey: apiKey,
		Model:  "claude-3-5-sonnet-20241022",
	})
	if err != nil {
		t.Fatalf("Failed to create Anthropic adapter: %v", err)
	}

	t.Run("Summarize", func(t *testing.T) {
		summary, err := adapter.Summarize(context.Background(),
			"Anthropic is an AI safety company that develops Claude, a helpful AI assistant.")
		if err != nil {
			t.Fatalf("Failed to summarize: %v", err)
		}
		if summary == "" {
			t.Error("Summary is empty")
		}
		t.Logf("Summary: %s", summary)
	})

	t.Run("Extract", func(t *testing.T) {
		info, err := adapter.Extract(context.Background(),
			"Anthropic develops Claude, an AI assistant focused on being helpful and safe.",
			schema.DefaultSchema())
		if err != nil {
			t.Fatalf("Failed to extract: %v", err)
		}
		if info.Summary == "" {
			t.Error("Summary is empty")
		}
		t.Logf("Extracted: %+v", info)
	})
}
