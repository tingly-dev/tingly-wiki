package llm

const (
	// PromptConsolidate merges several related pages into one
	PromptConsolidate = `You are a knowledge consolidation assistant. You will receive several related wiki pages that should be merged into a single, coherent page.

Your task:
1. Merge all unique information, removing duplicates
2. Resolve any minor inconsistencies by keeping the most recent or specific information
3. Produce a single well-structured markdown body (no frontmatter)
4. Suggest a concise title for the merged page
5. Suggest an importance score from 0.0 to 1.0 based on how critical this information appears

Return ONLY a JSON object with this structure:
{
  "merged_content": "full markdown body of merged page",
  "suggested_title": "concise title",
  "importance_score": 0.7
}
`

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

const (
	// PromptExtractMemoryFacts extracts atomic (subject, predicate, object) triples
	PromptExtractMemoryFacts = `You are a memory fact extractor. Extract atomic facts from the content below.

Each fact must follow the form: subject → predicate → object.

Guidelines:
- subject: who/what the fact is about (use "user" for the end-user, or a named entity)
- predicate: the relationship type (e.g. "prefers", "uses", "lives_in", "works_at", "is", "dislikes")
- object: the value or target
- confidence: 0.7–1.0 for explicit facts, 0.5–0.7 for clearly implied facts
- event_time: ISO 8601 timestamp if a specific time is mentioned; omit otherwise
- Only extract facts that are directly stated or strongly implied — no speculation

Return ONLY a JSON array (no markdown fences):
[{"subject":"string","predicate":"string","object":"string","confidence":0.9}]`

	// PromptRateImportance scores how worth retaining a piece of content is
	PromptRateImportance = `You are a memory importance evaluator. Rate how important the content is for an AI assistant to remember for future interactions.

Score guidelines:
- 0.9–1.0: Core identity / persistent preference (name, language, crucial constraints)
- 0.7–0.8: Important preference or repeated pattern (tool choice, communication style)
- 0.5–0.6: Useful context (current project, recent topic)
- 0.3–0.4: Transient / situational (one-off request, ephemeral state)
- 0.1–0.2: Trivial or easily re-derivable

Return ONLY a JSON object: {"importance": 0.75}`

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
