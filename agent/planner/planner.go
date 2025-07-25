package planner

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/santiagocorredoira/agent/agent/tools"
)

// TaskPlanner analyzes queries and decides which tools to use
type TaskPlanner struct {
	toolRegistry *tools.ToolRegistry
}

// NewTaskPlanner creates a new task planner
func NewTaskPlanner(registry *tools.ToolRegistry) *TaskPlanner {
	return &TaskPlanner{
		toolRegistry: registry,
	}
}

// AnalyzeQuery analyzes a user query and returns tool suggestions
func (p *TaskPlanner) AnalyzeQuery(query string) []ToolSuggestion {
	queryLower := strings.ToLower(query)
	var suggestions []ToolSuggestion

	// Priority 0: Tool listing - Check if user wants to know about available tools
	if p.shouldListTools(queryLower) {
		suggestions = append(suggestions, ToolSuggestion{
			ToolName: "tool_info",
			Priority: 110, // Highest priority
			Reason:   "User is asking about available tools",
			Parameters: map[string]interface{}{
				"list_all": true,
			},
		})
	}

	// Priority 1: Knowledge Base - Check for any technical/API questions or search requests
	if p.shouldSearchKnowledgeBase(queryLower) || p.shouldSearchLocalDocs(queryLower) {
		suggestions = append(suggestions, ToolSuggestion{
			ToolName: "kbase",
			Priority: 100, // Highest priority
			Reason:   "Query requires searching the knowledge base",
			Parameters: map[string]interface{}{
				"query":       query,
				"max_results": 5,
			},
		})
	}

	// Priority 3: File operations
	if p.shouldReadFile(queryLower) {
		suggestions = append(suggestions, ToolSuggestion{
			ToolName: "file_read",
			Priority: 80,
			Reason:   "Query requires reading a specific file",
		})
	}

	// Priority 4: Text search in files
	if p.shouldSearchText(queryLower) {
		suggestions = append(suggestions, ToolSuggestion{
			ToolName: "text_search",
			Priority: 70,
			Reason:   "Query requires searching for text patterns",
		})
	}

	// Priority 5: HTTP operations
	if p.shouldMakeHTTPRequest(queryLower) {
		suggestions = append(suggestions, ToolSuggestion{
			ToolName: "http_get",
			Priority: 60,
			Reason:   "Query requires fetching data from web",
		})
	}

	return suggestions
}

// shouldListTools determines if the user is asking about available tools
func (p *TaskPlanner) shouldListTools(query string) bool {
	toolKeywords := []string{
		"tools", "herramientas", "tool", "herramienta",
		"what tools", "qué herramientas", "que herramientas",
		"available tools", "herramientas disponibles",
		"what can you do", "qué puedes hacer", "que puedes hacer",
		"capabilities", "capacidades", "funciones",
		"what do you have", "qué tienes", "que tienes",
		"list tools", "lista herramientas", "listar herramientas",
		"show tools", "muestra herramientas", "mostrar herramientas",
		"available", "disponible", "disponibles",
	}

	for _, keyword := range toolKeywords {
		if strings.Contains(query, keyword) {
			return true
		}
	}

	return false
}

// shouldSearchKnowledgeBase determines if we should search the knowledge base
func (p *TaskPlanner) shouldSearchKnowledgeBase(query string) bool {
	// Keywords that indicate API/technical questions
	apiKeywords := []string{
		"api", "endpoint", "auth", "authentication", "webhook",
		"request", "response", "booking", "payment", "billing",
		"reservation", "booking", "cancel", "schedule",
		"how to", "how do", "what is", "explain", "documentation",
		"integrate", "integration", "method", "parameter", "field",
		"model", "schema", "format", "example", "sample",
		"error", "status", "code", "token", "credential",
		"configure", "configuration", "setup", "install",
	}

	for _, keyword := range apiKeywords {
		if strings.Contains(query, keyword) {
			return true
		}
	}

	// Check for question patterns
	questionStarters := []string{
		"cómo", "como", "qué", "que", "cuál", "cual",
		"dónde", "donde", "cuándo", "cuando", "por qué",
		"puedo", "necesito", "debo", "tengo que",
		"hay", "existe", "está", "esta",
	}

	for _, starter := range questionStarters {
		if strings.HasPrefix(query, starter) || strings.Contains(query, " "+starter+" ") {
			return true
		}
	}

	return false
}

// shouldSearchLocalDocs determines if we should search local documentation
func (p *TaskPlanner) shouldSearchLocalDocs(query string) bool {
	keywords := []string{
		"busca", "search", "encuentra", "find",
		"documentación", "documentation", "docs",
		"archivo", "file", "local",
	}

	for _, keyword := range keywords {
		if strings.Contains(query, keyword) {
			return true
		}
	}

	return false
}

// shouldReadFile determines if we should read a specific file
func (p *TaskPlanner) shouldReadFile(query string) bool {
	keywords := []string{
		"lee", "read", "muestra", "show",
		"contenido de", "content of",
		"archivo", "file",
	}

	// Check if query contains file path
	if strings.Contains(query, "/") || strings.Contains(query, "\\") {
		for _, keyword := range keywords {
			if strings.Contains(query, keyword) {
				return true
			}
		}
	}

	return false
}

// shouldSearchText determines if we should search for text patterns
func (p *TaskPlanner) shouldSearchText(query string) bool {
	keywords := []string{
		"grep", "buscar texto", "search text",
		"patrón", "pattern", "regex",
		"contiene", "contains",
	}

	for _, keyword := range keywords {
		if strings.Contains(query, keyword) {
			return true
		}
	}

	return false
}

// shouldMakeHTTPRequest determines if we should make HTTP requests
func (p *TaskPlanner) shouldMakeHTTPRequest(query string) bool {
	keywords := []string{
		"http", "https", "url", "fetch",
		"descarga", "download", "api call",
		"web", "website",
	}

	for _, keyword := range keywords {
		if strings.Contains(query, keyword) {
			return true
		}
	}

	return strings.Contains(query, "http://") || strings.Contains(query, "https://")
}

// ExecutePlan executes tool suggestions in priority order
func (p *TaskPlanner) ExecutePlan(ctx context.Context, suggestions []ToolSuggestion) ([]ToolExecutionResult, error) {
	if len(suggestions) == 0 {
		return nil, nil
	}

	// Sort by priority (highest first)
	for i := 0; i < len(suggestions)-1; i++ {
		for j := i + 1; j < len(suggestions); j++ {
			if suggestions[j].Priority > suggestions[i].Priority {
				suggestions[i], suggestions[j] = suggestions[j], suggestions[i]
			}
		}
	}

	var results []ToolExecutionResult

	// Execute tools in priority order
	for _, suggestion := range suggestions {
		// Check if tool exists
		_, exists := p.toolRegistry.GetTool(suggestion.ToolName)
		if !exists {
			continue
		}

		// Execute the tool
		execution := tools.ToolExecution{
			ToolName:   suggestion.ToolName,
			Parameters: suggestion.Parameters,
			Confirmed:  true, // Auto-confirm for planning
			Timestamp:  time.Now(),
		}

		result, err := p.toolRegistry.ExecuteTool(ctx, execution)
		if err != nil {
			results = append(results, ToolExecutionResult{
				ToolName: suggestion.ToolName,
				Success:  false,
				Error:    err.Error(),
			})
			continue
		}

		results = append(results, ToolExecutionResult{
			ToolName: suggestion.ToolName,
			Success:  result.Success,
			Result:   result,
		})

		// If knowledge search found results, stop here
		if suggestion.ToolName == "kbase" && result.Success {
			if data, ok := result.Data.(map[string]interface{}); ok {
				if totalResults, ok := data["total_results"].(int); ok && totalResults > 0 {
					break // Found relevant documentation, no need for other tools
				}
			}
		}
	}

	return results, nil
}

// ToolSuggestion represents a suggested tool to use
type ToolSuggestion struct {
	ToolName   string
	Priority   int
	Reason     string
	Parameters map[string]interface{}
}

// ToolExecutionResult represents the result of executing a tool
type ToolExecutionResult struct {
	ToolName string
	Success  bool
	Result   *tools.ToolResult
	Error    string
}

// FormatResultsForContext formats tool results for LLM context
func (p *TaskPlanner) FormatResultsForContext(results []ToolExecutionResult) string {
	if len(results) == 0 {
		return ""
	}

	var context strings.Builder
	context.WriteString("Based on searching the knowledge base and available tools:\n\n")

	for _, result := range results {
		if !result.Success {
			continue
		}

		if result.ToolName == "kbase" && result.Result != nil {
			context.WriteString("📚 Documentation Found:\n")
			context.WriteString(result.Result.Message)
			context.WriteString("\n\n")

			// Add relevant content from results
			if data, ok := result.Result.Data.(map[string]interface{}); ok {
				if results, ok := data["results"].([]map[string]interface{}); ok {
					for i, doc := range results {
						if i >= 3 { // Limit to top 3
							break
						}
						if content, ok := doc["relevant_content"].(string); ok {
							context.WriteString(fmt.Sprintf("From %s:\n%s\n\n",
								doc["title"], content))
						}
					}
				}
			}
		} else if result.Result != nil {
			context.WriteString(fmt.Sprintf("Tool %s: %s\n",
				result.ToolName, result.Result.Message))
		}
	}

	return context.String()
}
