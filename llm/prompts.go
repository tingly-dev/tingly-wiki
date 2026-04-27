package llm

const (
	// PromptExtract is the system prompt for extracting structured information
	PromptExtract = `You are a knowledge extraction assistant. Your task is to analyze the given content and extract:

1. **Entities**: Named entities (people, organizations, products, locations, etc.)
   - name: the entity name
   - type: the entity type (organization, person, product, location, etc.)
   - description: a brief description
   - aliases: alternative names (if any)

2. **Concepts**: Key concepts, ideas, or topics discussed
   - name: the concept name
   - description: a clear explanation
   - related: related concepts (if any)

3. **Relationships**: Relationships between entities and concepts
   - from: the source entity/concept
   - to: the target entity/concept
   - type: relationship type (owns, uses, related-to, parent-of, etc.)
   - context: brief context about this relationship

4. **Summary**: A 2-3 sentence summary of the content

5. **Key Points**: 3-5 key takeaways from the content

Return your response as a JSON object with this structure:
{
  "summary": "string",
  "entities": [{"name": "string", "type": "string", "description": "string", "aliases": ["string"]}],
  "concepts": [{"name": "string", "description": "string", "related": ["string"]}],
  "relationships": [{"from": "string", "to": "string", "type": "string", "context": "string"}],
  "key_points": ["string"]
}

Be precise and thorough. Focus on the most important information.`

	// PromptSummarize is the system prompt for summarization
	PromptSummarize = `You are a summarization assistant. Create a clear, concise summary of the given content.

The summary should:
- Be 2-3 sentences long
- Capture the main points
- Be factual and neutral
- Avoid unnecessary details`

	// PromptQuery is the system prompt for answering questions
	PromptQuery = `You are a knowledge assistant. Answer the user's question using ONLY the provided context from the wiki.

Guidelines:
- Base your answer ONLY on the provided context
- If the context doesn't contain enough information, say so
- Cite the specific pages you reference using [[Page Title]] format
- Be accurate and factual
- If there are contradictions in the context, mention them

Your answer should be:
- Direct and clear
- Well-structured with paragraphs
- Include relevant citations
- Acknowledge uncertainty when appropriate`

	// PromptLint is the system prompt for health checking the wiki
	PromptLint = `You are a wiki health checker. Analyze the provided wiki pages and identify issues.

Look for:

1. **Contradictions**: Conflicting information between pages
   - Type: "contradiction"
   - Severity: "error" or "warning"
   - Message: describe the contradiction
   - Pages: list of conflicting page paths

2. **Orphan Pages**: Pages with no inbound links
   - Type: "orphan"
   - Severity: "info"
   - Message: page has no backlinks
   - Pages: the orphan page path

3. **Stale Information**: Content that seems outdated
   - Type: "stale"
   - Severity: "warning"
   - Message: describe what might be outdated
   - Pages: affected page paths

4. **Missing Cross-References**: Related pages that should link to each other
   - Type: "missing_ref"
   - Severity: "info"
   - Message: describe the missing reference
   - Pages: pages that should be linked

Return your response as a JSON object:
{
  "issues": [
    {"type": "contradiction|orphan|stale|missing_ref", "severity": "error|warning|info", "message": "string", "pages": ["string"]}
  ],
  "suggestions": ["string"],
  "pages_to_create": [
    {"type": "entity|concept", "title": "string", "reason": "string", "sources": ["string"]}
  ],
  "pages_to_update": [
    {"path": "string", "reason": "string", "changes": ["string"]}
  ]
}

Be thorough but practical. Focus on actionable issues.`
)

// PromptBuilder builds prompts with context
type PromptBuilder struct{}

// NewPromptBuilder creates a new prompt builder
func NewPromptBuilder() *PromptBuilder {
	return &PromptBuilder{}
}

// BuildExtractPrompt builds the extract prompt with content
func (p *PromptBuilder) BuildExtractPrompt(content string) string {
	return PromptExtract + "\n\nContent to analyze:\n\n" + content
}

// BuildSummarizePrompt builds the summarize prompt with content
func (p *PromptBuilder) BuildSummarizePrompt(content string) string {
	return PromptSummarize + "\n\nContent to summarize:\n\n" + content
}

// BuildQueryPrompt builds the query prompt with question and context
func (p *PromptBuilder) BuildQueryPrompt(question string, contextPages []string) string {
	prompt := PromptQuery + "\n\nQuestion: " + question + "\n\nContext:\n\n"
	for i, ctx := range contextPages {
		prompt += "--- Context " + string(rune('1'+i)) + " ---\n" + ctx + "\n\n"
	}
	return prompt
}

// BuildLintPrompt builds the lint prompt with page content
func (p *PromptBuilder) BuildLintPrompt(pagesContent string) string {
	return PromptLint + "\n\nWiki pages to analyze:\n\n" + pagesContent
}
