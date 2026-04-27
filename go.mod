module github.com/tingly-dev/tingly-wiki

go 1.21

require (
	github.com/google/uuid v1.6.0
	gopkg.in/yaml.v3 v3.0.1
)

// Use local SDKs from parent monorepo
replace github.com/openai/openai-go/v3 => ../libs/openai-go

replace github.com/anthropics/anthropic-sdk-go => ../libs/anthropic-sdk-go
