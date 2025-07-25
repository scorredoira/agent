package tools

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/santiagocorredoira/agent/agent/llm"
)

// FileSearchTool searches for files using glob patterns
type FileSearchTool struct{
	llmProvider llm.Provider
}

func NewFileSearchTool(llmProvider llm.Provider) *FileSearchTool {
	return &FileSearchTool{
		llmProvider: llmProvider,
	}
}

func (t *FileSearchTool) GetName() string {
	return "file_search"
}

func (t *FileSearchTool) GetDescription() string {
	return "Search for files using glob patterns within the kbase directory only. Use broad patterns to find documentation that might contain your information (e.g., use '*customer*' to find files about customer fields, not '*memberCode*'). Access is restricted to kbase for security."
}

func (t *FileSearchTool) GetCategory() ToolCategory {
	return CategoryData
}

func (t *FileSearchTool) RequiresConfirmation() bool {
	return false
}

func (t *FileSearchTool) GetEstimatedCost() int {
	return 1
}

func (t *FileSearchTool) GetFunctionDefinition() llm.FunctionDefinition {
	return DefaultGetFunctionDefinition(t)
}

func (t *FileSearchTool) GetParameterSchema() *ParameterSchema {
	return &ParameterSchema{
		Type: "object",
		Properties: map[string]PropertySchema{
			"pattern": {
				Type:        "string",
				Description: "Glob pattern to search for files. Use broad patterns without file extensions (e.g. '**/*customer*' finds all files containing 'customer')",
			},
			"path": {
				Type:        "string",
				Description: "Base path to search from within kbase directory (default: ./kbase). Must be inside kbase.",
				Default:     "./kbase",
			},
		},
		Required:    []string{"pattern"},
		Description: "Search for files matching a pattern",
	}
}

func (t *FileSearchTool) IsAvailable(ctx context.Context) bool {
	return true
}

func (t *FileSearchTool) Execute(ctx context.Context, params map[string]interface{}) (*ToolResult, error) {
	pattern, ok := params["pattern"].(string)
	if !ok || pattern == "" {
		return &ToolResult{
			Success: false,
			Error:   "pattern parameter is required",
		}, nil
	}

	// SECURITY: Restrict to kbase directory only
	basePath := "./kbase"
	if pathParam, exists := params["path"]; exists {
		if p, ok := pathParam.(string); ok {
			// Validate path - must be within kbase
			cleanPath := filepath.Clean(p)
			absPath, err := filepath.Abs(cleanPath)
			if err != nil {
				return &ToolResult{
					Success: false,
					Error:   "Invalid path provided",
				}, nil
			}

			kbPath, err := filepath.Abs("./kbase")
			if err != nil {
				return &ToolResult{
					Success: false,
					Error:   "Cannot resolve kbase path",
				}, nil
			}

			// Ensure the requested path is within kbase
			if !strings.HasPrefix(absPath, kbPath) {
				return &ToolResult{
					Success: false,
					Error:   "Access denied: file_search is restricted to kbase directory only",
				}, nil
			}
			basePath = cleanPath
		}
	}

	// Build the full pattern path
	var fullPattern string
	if basePath == "./kbase" {
		fullPattern = filepath.Join(basePath, pattern)
	} else {
		fullPattern = filepath.Join(basePath, pattern)
	}

	// Find matching files with custom implementation for ** support
	matches, err := t.findMatches(fullPattern)
	if err != nil {
		return &ToolResult{
			Success: false,
			Error:   fmt.Sprintf("Pattern search failed: %v", err),
		}, nil
	}

	// Get general documents that are relevant to the search
	generalDocs, err := t.getRelevantGeneralDocs(ctx, pattern)
	if err != nil {
		// Log error but don't fail the search
		fmt.Printf("Warning: Failed to get general docs: %v\n", err)
	}

	if len(matches) == 0 && len(generalDocs) == 0 {
		return &ToolResult{
			Success: true,
			Message: fmt.Sprintf("No files found matching pattern '%s'", fullPattern),
			Data: map[string]interface{}{
				"pattern": fullPattern,
				"path":    basePath,
				"matches": []string{},
				"general_docs": []map[string]interface{}{},
			},
		}, nil
	}

	// Build result message
	var message strings.Builder
	
	if len(matches) > 0 {
		message.WriteString(fmt.Sprintf("Found %d files matching '%s':\n", len(matches), fullPattern))
		for i, match := range matches {
			if i > 50 { // Limit output
				message.WriteString(fmt.Sprintf("\n... and %d more files", len(matches)-i))
				break
			}
			message.WriteString(fmt.Sprintf("- %s\n", match))
		}
	}

	if len(generalDocs) > 0 {
		if len(matches) > 0 {
			message.WriteString("\n")
		}
		message.WriteString(fmt.Sprintf("Additionally, found %d relevant general documentation files:\n", len(generalDocs)))
		for _, doc := range generalDocs {
			message.WriteString(fmt.Sprintf("- %s (General Documentation)\n", doc["file_path"]))
		}
	}

	return &ToolResult{
		Success: true,
		Message: message.String(),
		Data: map[string]interface{}{
			"pattern": fullPattern,
			"path":    basePath,
			"matches": matches,
			"count":   len(matches),
			"general_docs": generalDocs,
			"general_count": len(generalDocs),
		},
	}, nil
}

// findMatches implements glob pattern matching with ** support
func (t *FileSearchTool) findMatches(pattern string) ([]string, error) {
	var matches []string

	// SECURITY: Ensure we only search within kbase
	kbPath, err := filepath.Abs("./kbase")
	if err != nil {
		return nil, fmt.Errorf("cannot resolve kbase path: %v", err)
	}

	// If pattern doesn't contain **, use regular glob but filter files only
	if !strings.Contains(pattern, "**") {
		globMatches, err := filepath.Glob(pattern)
		if err != nil {
			return nil, err
		}

		// Filter to files only (no directories) and within kbase
		for _, match := range globMatches {
			absMatch, err := filepath.Abs(match)
			if err != nil {
				continue
			}

			// Security check: ensure match is within kbase
			if !strings.HasPrefix(absMatch, kbPath) {
				continue
			}

			if info, err := os.Stat(match); err == nil && !info.IsDir() {
				matches = append(matches, match)
			}
		}
		return matches, nil
	}

	// Handle ** patterns by walking the entire directory tree
	parts := strings.Split(pattern, "**")
	if len(parts) != 2 {
		// Multiple ** not supported, fall back to regular glob
		return filepath.Glob(pattern)
	}

	prefix := parts[0]
	suffix := parts[1]

	// Clean up prefix and suffix
	prefix = strings.TrimSuffix(prefix, "/")
	suffix = strings.TrimPrefix(suffix, "/")

	// If prefix is empty, start from current directory
	if prefix == "" {
		prefix = "."
	}

	// SECURITY: Ensure prefix is within kbase
	absPrefix, err := filepath.Abs(prefix)
	if err != nil {
		return nil, fmt.Errorf("cannot resolve prefix path: %v", err)
	}

	if !strings.HasPrefix(absPrefix, kbPath) {
		return nil, fmt.Errorf("access denied: search path must be within kbase directory")
	}

	// Walk the directory tree
	err = filepath.WalkDir(prefix, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // Skip errors, don't fail the whole search
		}

		// Skip directories
		if d.IsDir() {
			return nil
		}

		// SECURITY: Double-check each file is within kbase
		absPath, err := filepath.Abs(path)
		if err != nil {
			return nil
		}

		if !strings.HasPrefix(absPath, kbPath) {
			return nil // Skip files outside kbase
		}

		// Get relative path from prefix for matching
		relPath, err := filepath.Rel(prefix, path)
		if err != nil {
			return nil
		}

		// If suffix is empty (pattern ends with **), match all files
		if suffix == "" {
			matches = append(matches, path)
			return nil
		}

		// Match against the suffix pattern
		var matched bool

		// If suffix contains path separators, match against relative path
		if strings.Contains(suffix, "/") {
			var matchErr error
			matched, matchErr = filepath.Match(suffix, relPath)
			if matchErr != nil {
				return nil
			}
		} else {
			// If suffix is just a filename pattern, match against filename only
			filename := filepath.Base(path)
			var matchErr error
			matched, matchErr = filepath.Match(suffix, filename)
			if matchErr != nil {
				return nil
			}
		}

		if matched {
			matches = append(matches, path)
		}

		return nil
	})

	return matches, err
}

// getRelevantGeneralDocs finds general documents relevant to the search query
func (t *FileSearchTool) getRelevantGeneralDocs(ctx context.Context, searchQuery string) ([]map[string]interface{}, error) {
	if t.llmProvider == nil {
		return []map[string]interface{}{}, nil
	}

	// Find all general documents in kbase
	generalDocs, err := t.findGeneralDocuments()
	if err != nil {
		return nil, fmt.Errorf("failed to find general documents: %v", err)
	}

	if len(generalDocs) == 0 {
		return []map[string]interface{}{}, nil
	}

	// Use LLM to evaluate relevance
	evaluator := NewDocumentRelevanceEvaluator(t.llmProvider)
	
	var result []map[string]interface{}
	
	for _, doc := range generalDocs {
		relevant, err := evaluator.EvaluateRelevance(ctx, searchQuery, doc.FilePath)
		if err != nil {
			// Skip this document but continue with others
			continue
		}
		
		if relevant {
			// Read full content of relevant documents
			content, err := GetDocumentContent(doc.FilePath)
			if err != nil {
				// Skip if we can't read the content
				continue
			}
			
			result = append(result, map[string]interface{}{
				"file_path": doc.FilePath,
				"title": doc.Title,
				"group": doc.Group,
				"keywords": doc.Keywords,
				"priority": doc.Priority,
				"content": content,
			})
		}
	}
	
	return result, nil
}

// findGeneralDocuments searches for all documents with keywords="general"
func (t *FileSearchTool) findGeneralDocuments() ([]*DocumentMetadata, error) {
	parser := NewDocumentParser()
	var generalDocs []*DocumentMetadata
	
	// Walk through all .md files in kbase
	kbPath := "./kbase"
	err := filepath.WalkDir(kbPath, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // Skip errors
		}
		
		// Only process .md files
		if d.IsDir() || !strings.HasSuffix(strings.ToLower(path), ".md") {
			return nil
		}
		
		// Parse metadata
		metadata, err := parser.ParseMetadata(path)
		if err != nil {
			return nil // Skip files with parse errors
		}
		
		// Check if it has "general" keyword
		if metadata.HasKeyword("general") {
			generalDocs = append(generalDocs, metadata)
		}
		
		return nil
	})
	
	return generalDocs, err
}
