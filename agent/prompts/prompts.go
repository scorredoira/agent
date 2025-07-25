package prompts

import (
	_ "embed"
	"fmt"
	"strings"
)

// Embedded prompt templates
//go:embed document_relevance.md
var DocumentRelevanceTemplate string

//go:embed system_brief.md
var SystemBriefTemplate string

//go:embed conversation_title.md
var ConversationTitleTemplate string

//go:embed conversation_summary.md
var ConversationSummaryTemplate string

//go:embed system_base.md
var SystemBaseTemplate string

//go:embed search_strategy.md
var SearchStrategyTemplate string

//go:embed filter_rules.md
var FilterRulesTemplate string

//go:embed anti_hallucination.md
var AntiHallucinationTemplate string

//go:embed tools_only_mode.md
var ToolsOnlyModeTemplate string

// PromptData represents data to substitute in prompts
type PromptData struct {
	SearchQuery      string
	FileName         string
	DirName          string
	FilePath         string
	FirstUserMessage string
	Messages         string
	ToolCount        int
}

// RenderDocumentRelevancePrompt renders the document relevance prompt with data
func RenderDocumentRelevancePrompt(data PromptData) string {
	prompt := DocumentRelevanceTemplate
	prompt = strings.ReplaceAll(prompt, "{{.SearchQuery}}", data.SearchQuery)
	prompt = strings.ReplaceAll(prompt, "{{.FileName}}", data.FileName)
	prompt = strings.ReplaceAll(prompt, "{{.DirName}}", data.DirName)
	prompt = strings.ReplaceAll(prompt, "{{.FilePath}}", data.FilePath)
	return prompt
}

// RenderConversationTitlePrompt renders the conversation title prompt with data
func RenderConversationTitlePrompt(data PromptData) string {
	prompt := ConversationTitleTemplate
	prompt = strings.ReplaceAll(prompt, "{{.FirstUserMessage}}", data.FirstUserMessage)
	return prompt
}

// RenderConversationSummaryPrompt renders the conversation summary prompt with data
func RenderConversationSummaryPrompt(data PromptData) string {
	prompt := ConversationSummaryTemplate
	prompt = strings.ReplaceAll(prompt, "{{.Messages}}", data.Messages)
	return prompt
}

// RenderSystemBasePrompt renders the base system prompt with data
func RenderSystemBasePrompt(data PromptData) string {
	prompt := SystemBaseTemplate
	prompt = strings.ReplaceAll(prompt, "{{.ToolCount}}", fmt.Sprintf("%d", data.ToolCount))
	return prompt
}

// GetSearchStrategyPrompt returns the search strategy prompt
func GetSearchStrategyPrompt() string {
	return SearchStrategyTemplate
}

// GetFilterRulesPrompt returns the filter rules prompt
func GetFilterRulesPrompt() string {
	return FilterRulesTemplate
}

// GetAntiHallucinationPrompt returns the anti-hallucination rules prompt
func GetAntiHallucinationPrompt() string {
	return AntiHallucinationTemplate
}

// GetToolsOnlyModePrompt returns the tools-only mode prompt
func GetToolsOnlyModePrompt() string {
	return ToolsOnlyModeTemplate
}