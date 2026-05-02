package config

import (
	"fmt"
	"os"
	"regexp"

	"gopkg.in/yaml.v3"
)

// envVarRE matches ${VAR_NAME} patterns for environment variable expansion
var envVarRE = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)\}`)

// CLIConfig is the CLI configuration loaded from file
type CLIConfig struct {
	// Wiki directory
	WikiDir string `yaml:"wiki_dir"`

	// LLM Configuration
	LLM LLMConfig `yaml:"llm"`

	// Benchmark configuration
	Benchmark BenchmarkConfig `yaml:"benchmark"`
}

// LLMConfig holds LLM-related configuration
type LLMConfig struct {
	// Provider: openai, anthropic, mock
	Provider string `yaml:"provider"`

	// OpenAI-specific settings
	OpenAI OpenAIConfig `yaml:"openai"`

	// Anthropic-specific settings
	Anthropic AnthropicConfig `yaml:"anthropic"`
}

// OpenAIConfig holds OpenAI configuration
type OpenAIConfig struct {
	APIKey  string `yaml:"api_key"`
	BaseURL string `yaml:"base_url"`
	// Chat model (gpt-4o-mini, gpt-4o, etc.)
	Model string `yaml:"model"`
	// Embedding model (text-embedding-3-small, text-embedding-3-large, etc.)
	EmbeddingModel string `yaml:"embedding_model"`
}

// AnthropicConfig holds Anthropic configuration
type AnthropicConfig struct {
	APIKey string `yaml:"api_key"`
	Model  string `yaml:"model"`
}

// BenchmarkConfig holds benchmark-specific configuration
type BenchmarkConfig struct {
	// Data directory for benchmark datasets
	DataDir string `yaml:"data_dir"`
	// Default dataset
	Dataset string `yaml:"dataset"`
	// Use real LLM for embeddings
	RealLLM bool `yaml:"real_llm"`
	// Output format
	Format string `yaml:"format"`
}

// LoadCLIConfig loads CLI configuration from a YAML file
// Searches in order: .wiki/config.yml, .wiki.yaml, ~/.wiki/config.yml, ./config.yml
func LoadCLIConfig(wikiDir string, explicitPath string) (*CLIConfig, error) {
	configPaths := []string{}

	if explicitPath != "" {
		configPaths = append(configPaths, explicitPath)
	} else {
		// Default search paths
		if wikiDir != "" {
			configPaths = append(configPaths,
				wikiDir+"/config.yml",
				wikiDir+"/.wiki.yml",
			)
		}
		// Home directory config
		homeDir, _ := os.UserHomeDir()
		if homeDir != "" {
			configPaths = append(configPaths,
				homeDir+"/.wiki/config.yml",
			)
		}
		// Current directory
		configPaths = append(configPaths, "./config.yml")
	}

	// Try each path
	for _, path := range configPaths {
		if cfg, err := loadConfigFile(path); err == nil {
			return cfg, nil
		}
	}

	// Return default config if none found
	return DefaultCLIConfig(), nil
}

// loadConfigFile attempts to load a config file from the given path
func loadConfigFile(path string) (*CLIConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	// Expand environment variables
	data = []byte(expandEnvVars(string(data)))

	var cfg CLIConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}

	return &cfg, nil
}

// expandEnvVars expands ${VAR_NAME} patterns with environment variables
func expandEnvVars(s string) string {
	return envVarRE.ReplaceAllStringFunc(s, func(match string) string {
		// Extract variable name without ${ and }
		varName := match[2 : len(match)-1]
		if val := os.Getenv(varName); val != "" {
			return val
		}
		return match // Keep original if env var not found
	})
}

// DefaultCLIConfig returns a default CLI configuration
func DefaultCLIConfig() *CLIConfig {
	return &CLIConfig{
		WikiDir: ".wiki",
		LLM: LLMConfig{
			Provider: "openai",
			OpenAI: OpenAIConfig{
				Model:          "gpt-4o-mini",
				EmbeddingModel: "text-embedding-3-small",
			},
		},
		Benchmark: BenchmarkConfig{
			DataDir: ".benchmark",
			Dataset: "locomo",
			RealLLM: false,
			Format:  "markdown",
		},
	}
}

// Save saves the CLI configuration to a file
func (c *CLIConfig) Save(path string) error {
	data, err := yaml.Marshal(c)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}

	return nil
}

// GetOpenAIConfig returns the OpenAI configuration
func (c *CLIConfig) GetOpenAIConfig() OpenAIConfig {
	return c.LLM.OpenAI
}

// GetWikiDir returns the wiki directory
func (c *CLIConfig) GetWikiDir() string {
	if c.WikiDir != "" {
		return c.WikiDir
	}
	return ".wiki"
}
