package tools

import (
	"context"
	"fmt"
	"io/fs"
	"strings"

	search_engine "textSearch"

	"github.com/santiagocorredoira/agent/agent/llm"
)

// SearchEngineTool wraps the generic search engine library
type SearchEngineTool struct {
	engine search_engine.SearchEngine
	llmProvider llm.Provider
}

// SearchEngineResult represents a file found by the search engine
type SearchEngineResult struct {
	FilePath string  `json:"file_path"`
	Score    float64 `json:"score"`
	Reason   string  `json:"reason"`
	FileName string  `json:"filename"`
}

// NewSearchEngineTool creates a new search engine tool with a restricted filesystem
func NewSearchEngineTool(restrictedFS fs.FS) *SearchEngineTool {
	return &SearchEngineTool{
		engine: search_engine.NewSearchEngine(restrictedFS),
	}
}

// NewSearchEngineToolWithLLM creates a new search engine tool with LLM provider for relevance evaluation
func NewSearchEngineToolWithLLM(restrictedFS fs.FS, llmProvider llm.Provider) *SearchEngineTool {
	return &SearchEngineTool{
		engine: search_engine.NewSearchEngine(restrictedFS),
		llmProvider: llmProvider,
	}
}

func (t *SearchEngineTool) GetName() string {
	return "kbase"
}

func (t *SearchEngineTool) GetDescription() string {
	return "Search for relevant files in the knowledge base using advanced semantic matching. Finds the most relevant documentation files for any query."
}

func (t *SearchEngineTool) GetCategory() ToolCategory {
	return CategoryData
}

func (t *SearchEngineTool) RequiresConfirmation() bool {
	return false
}

func (t *SearchEngineTool) GetEstimatedCost() int {
	return 1
}

func (t *SearchEngineTool) GetFunctionDefinition() llm.FunctionDefinition {
	return DefaultGetFunctionDefinition(t)
}

func (t *SearchEngineTool) GetParameterSchema() *ParameterSchema {
	return &ParameterSchema{
		Type: "object",
		Properties: map[string]PropertySchema{
			"query": {
				Type:        "string",
				Description: "Search query to find relevant documentation files",
			},
			"max_results": {
				Type:        "number",
				Description: "Maximum number of files to return",
				Default:     10,
				Minimum:     func() *float64 { v := 1.0; return &v }(),
				Maximum:     func() *float64 { v := 50.0; return &v }(),
			},
		},
		Required:    []string{"query"},
		Description: "Search for relevant files in the knowledge base",
	}
}

func (t *SearchEngineTool) IsAvailable(ctx context.Context) bool {
	return true
}

func (t *SearchEngineTool) Execute(ctx context.Context, params map[string]interface{}) (*ToolResult, error) {
	query, ok := params["query"].(string)
	if !ok || query == "" {
		return &ToolResult{
			Success: false,
			Error:   "query parameter is required",
		}, nil
	}

	maxResults := 10
	if maxParam, exists := params["max_results"]; exists {
		if max, ok := maxParam.(float64); ok {
			maxResults = int(max)
		}
	}

	// Use search engine to find relevant files
	files, err := t.engine.FindRelevantFiles(query, maxResults)
	if err != nil {
		return &ToolResult{
			Success: false,
			Error:   fmt.Sprintf("Search failed: %v", err),
		}, nil
	}

	// Convert to SearchEngineResult format
	results := make([]SearchEngineResult, len(files))
	for i, file := range files {
		results[i] = SearchEngineResult{
			FilePath: file.Path,
			Score:    file.Score,
			Reason:   file.Reason,
			FileName: file.FileName,
		}
	}

	// Get available general documentation using LLM-based relevance
	generalDocs, err := t.getRelevantGeneralDocsWithLLM(ctx, query)
	if err != nil {
		// Don't fail the whole request if general docs search fails
		generalDocs = []GeneralDoc{}
	}

	// Format message including general docs and search suggestions
	message := t.formatResults(query, results, generalDocs)
	
	// If results are poor, suggest alternative search terms
	if len(results) == 0 || (len(results) > 0 && results[0].Score < 0.3) {
		suggestions := t.generateSearchSuggestions(query)
		if len(suggestions) > 0 {
			message += fmt.Sprintf("\n\nSUGGESTED ALTERNATIVE SEARCHES:\nTry these terms: %s", strings.Join(suggestions, ", "))
		}
	}

	return &ToolResult{
		Success: true,
		Message: message,
		Data: map[string]interface{}{
			"query":         query,
			"total_results": len(results),
			"results":       results,
			"general_docs":  generalDocs,
		},
	}, nil
}

// ExtractContent extracts relevant content from a specific file
func (t *SearchEngineTool) ExtractContent(filePath, query string) (string, error) {
	return t.engine.ExtractRelevantContent(filePath, query, 1000)
}

// ReadFile reads the complete content of a file
func (t *SearchEngineTool) ReadFile(filePath string) (string, error) {
	return t.engine.GetFileContent(filePath)
}

// generateSearchSuggestions generates alternative search terms based on common patterns
func (t *SearchEngineTool) generateSearchSuggestions(query string) []string {
	query = strings.ToLower(strings.TrimSpace(query))
	suggestions := []string{}
	
	// Common API operation patterns
	if strings.Contains(query, "pay") || strings.Contains(query, "payment") {
		suggestions = append(suggestions, "paySales", "billing pay", "payment method", "sale payment")
	}
	
	if strings.Contains(query, "customer") || strings.Contains(query, "client") {
		suggestions = append(suggestions, "customer endpoint", "get customers", "customer API", "read customers")
	}
	
	if strings.Contains(query, "invoice") || strings.Contains(query, "factura") {
		suggestions = append(suggestions, "invoice endpoint", "create invoice", "invoice API", "billing invoice")
	}
	
	if strings.Contains(query, "sale") || strings.Contains(query, "venta") {
		suggestions = append(suggestions, "sale endpoint", "save sale", "sales API", "billing sale")
	}
	
	if strings.Contains(query, "booking") || strings.Contains(query, "reservation") {
		suggestions = append(suggestions, "booking endpoint", "cancel booking", "booking API", "save booking")
	}
	
	// Generic API patterns
	words := strings.Fields(query)
	for _, word := range words {
		// Try with endpoint suffix
		suggestions = append(suggestions, word+" endpoint")
		// Try with API suffix  
		suggestions = append(suggestions, word+" API")
		// Try with common verbs
		if !strings.HasPrefix(word, "get") && !strings.HasPrefix(word, "create") {
			suggestions = append(suggestions, "get "+word, "create "+word, "update "+word, "delete "+word)
		}
	}
	
	// Remove duplicates and limit
	seen := make(map[string]bool)
	unique := []string{}
	for _, s := range suggestions {
		if !seen[s] && s != query {
			seen[s] = true
			unique = append(unique, s)
		}
	}
	
	// Limit to 6 suggestions
	if len(unique) > 6 {
		unique = unique[:6]
	}
	
	return unique
}

// GeneralDoc represents a general documentation file with keywords
type GeneralDoc struct {
	FilePath string   `json:"file_path"`
	Title    string   `json:"title"`
	Keywords []string `json:"keywords"`
}

// GetAvailableGeneralDocs finds general documentation files with relevant keywords
func (t *SearchEngineTool) GetAvailableGeneralDocs(query string) ([]GeneralDoc, error) {
	// Search for files with keywords attribute in apidoc tags
	files, err := t.engine.FindRelevantFiles("apidoc keywords", 50) // Search more broadly
	if err != nil {
		return nil, err
	}

	var generalDocs []GeneralDoc
	for _, file := range files {
		// Extract first few lines to check for apidoc tag with keywords
		content, err := t.engine.ExtractRelevantContent(file.Path, "apidoc keywords", 5)
		if err != nil {
			continue
		}

		// Parse apidoc tag for keywords
		if keywords := extractKeywordsFromApidoc(content, query); len(keywords) > 0 {
			title := extractTitleFromApidoc(content)
			generalDocs = append(generalDocs, GeneralDoc{
				FilePath: file.Path,
				Title:    title,
				Keywords: keywords,
			})
		}
	}

	return generalDocs, nil
}

// Helper function to extract keywords from apidoc tag if they match query
func extractKeywordsFromApidoc(content, query string) []string {
	// Simple regex to find keywords attribute in apidoc tag
	// Look for: keywords="word1,word2,word3"
	start := strings.Index(content, `keywords="`)
	if start == -1 {
		return nil
	}
	start += len(`keywords="`)
	
	end := strings.Index(content[start:], `"`)
	if end == -1 {
		return nil
	}
	
	keywordsStr := content[start : start+end]
	keywords := strings.Split(keywordsStr, ",")
	
	// Clean and filter keywords that are relevant to query
	var relevantKeywords []string
	queryLower := strings.ToLower(query)
	queryWords := strings.Fields(queryLower) // Split query into words
	
	for _, keyword := range keywords {
		keyword = strings.TrimSpace(keyword)
		keywordLower := strings.ToLower(keyword)
		if keyword == "" {
			continue
		}
		
		// Check if keyword matches query (bidirectional matching)
		isRelevant := false
		
		// Check if query contains keyword
		if strings.Contains(queryLower, keywordLower) {
			isRelevant = true
		}
		
		// Check if keyword contains any query word (for partial matches)
		for _, word := range queryWords {
			if len(word) > 3 && strings.Contains(keywordLower, word) {
				isRelevant = true
				break
			}
		}
		
		// Check if any query word contains keyword (for Spanish-English variations)
		for _, word := range queryWords {
			if len(word) > 3 && strings.Contains(word, keywordLower) {
				isRelevant = true
				break
			}
		}
		
		if isRelevant {
			relevantKeywords = append(relevantKeywords, keyword)
		}
	}
	
	return relevantKeywords
}

// Helper function to extract title from apidoc tag
func extractTitleFromApidoc(content string) string {
	start := strings.Index(content, `title="`)
	if start == -1 {
		return "General Documentation"
	}
	start += len(`title="`)
	
	end := strings.Index(content[start:], `"`)
	if end == -1 {
		return "General Documentation"
	}
	
	return content[start : start+end]
}

func (t *SearchEngineTool) formatResults(query string, results []SearchEngineResult, generalDocs []GeneralDoc) string {
	if len(results) == 0 {
		return fmt.Sprintf("No relevant files found for '%s' in knowledge base.", query)
	}

	message := fmt.Sprintf("Found %d relevant files for '%s':\n\n", len(results), query)

	for i, result := range results {
		if i >= 3 { // Show top 3 with details and content
			break
		}
		message += fmt.Sprintf("%d. %s (Score: %.2f)\n",
			i+1, result.FileName, result.Score)
		if result.Reason != "" {
			message += fmt.Sprintf("   Reason: %s\n", result.Reason)
		}

		// Extract and show relevant content
		content, err := t.engine.ExtractRelevantContent(result.FilePath, query, 1000)
		if err == nil && len(content) > 0 {
			// Allow much larger content to ensure we capture filtering sections
			if len(content) > 5000 {
				content = content[:5000] + "..."
			}
			message += fmt.Sprintf("   Content: %s\n", content)
		}
		message += "\n"
	}

	if len(results) > 3 {
		message += fmt.Sprintf("... and %d more files available\n", len(results)-3)
	}

	// Add general documentation with COMPLETE content
	if len(generalDocs) > 0 {
		message += "\n📚 RELEVANT GENERAL DOCUMENTATION:\n\n"
		for _, doc := range generalDocs {
			message += fmt.Sprintf("=== %s (relevant keywords: %s) ===\n", doc.Title, strings.Join(doc.Keywords, ", "))
			
			// Read COMPLETE content of the general documentation file  
			// Remove kbase/ prefix since engine already operates within kbase
			relativePath := strings.TrimPrefix(doc.FilePath, "kbase/")
			fullContent, err := t.engine.GetFileContent(relativePath)
			if err == nil && fullContent != "" {
				message += fullContent + "\n\n"
			} else {
				message += "Error reading file content.\n\n"
			}
		}
	}

	return message
}

// getRelevantGeneralDocsWithLLM finds general documents relevant to the search query using LLM evaluation
func (t *SearchEngineTool) getRelevantGeneralDocsWithLLM(ctx context.Context, searchQuery string) ([]GeneralDoc, error) {
	if t.llmProvider == nil {
		// Fallback to the original method if no LLM provider
		return t.GetAvailableGeneralDocs(searchQuery)
	}

	// Find all general documents using the document parser
	generalDocs, err := t.findGeneralDocumentsWithParser()
	if err != nil {
		return nil, fmt.Errorf("failed to find general documents: %v", err)
	}

	if len(generalDocs) == 0 {
		return []GeneralDoc{}, nil
	}

	// Use LLM to evaluate relevance
	evaluator := NewDocumentRelevanceEvaluator(t.llmProvider)
	
	var result []GeneralDoc
	
	for _, doc := range generalDocs {
		relevant, err := evaluator.EvaluateRelevance(ctx, searchQuery, doc.FilePath)
		if err != nil {
			// Skip this document but continue with others
			continue
		}
		
		if relevant {
			result = append(result, doc)
		}
	}
	
	return result, nil
}

// findGeneralDocumentsWithParser searches for all documents with keywords="general" using the document parser
func (t *SearchEngineTool) findGeneralDocumentsWithParser() ([]GeneralDoc, error) {
	parser := NewDocumentParser()
	var generalDocs []GeneralDoc
	
	// Get all .md files from the search engine
	// Try different search patterns to find markdown files
	allFiles, err := t.engine.FindRelevantFiles("md", 1000) // Try without wildcard first
	if err != nil || len(allFiles) == 0 {
		allFiles, err = t.engine.FindRelevantFiles("apidoc", 1000) // Search for apidoc tag
		if err != nil {
			return nil, err
		}
	}
	
	for _, file := range allFiles {
		// Construct full path with kbase directory
		fullPath := fmt.Sprintf("kbase/%s", file.Path)
		
		// Parse metadata
		metadata, err := parser.ParseMetadata(fullPath)
		if err != nil {
			continue // Skip files with parse errors
		}
		
		// Check if it has "general" keyword
		if metadata.HasKeyword("general") {
			generalDocs = append(generalDocs, GeneralDoc{
				FilePath: fullPath,
				Title:    metadata.Title,
				Keywords: metadata.Keywords,
			})
		}
	}
	
	return generalDocs, nil
}
