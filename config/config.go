package config

// Config is the wiki configuration
type Config struct {
	// Layout defines directory structure
	Layout *LayoutConfig

	// Storage backend to use
	Storage interface{}

	// LLM backend to use
	LLM interface{}

	// Index backend to use
	Index interface{}

	// VectorIndex is an optional dense-vector index for semantic retrieval.
	// When nil, RecallMemory degrades to keyword-only search.
	VectorIndex interface{}

	// Concurrency settings
	MaxConcurrentIngests int
}

// DefaultConfig returns a default configuration
func DefaultConfig() *Config {
	return &Config{
		Layout:              DefaultLayout(),
		MaxConcurrentIngests: 1,
	}
}
