package tools

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/santiagocorredoira/agent/agent/llm"
	"github.com/santiagocorredoira/agent/agent/prompts"
)

// DocumentRelevanceEvaluator uses LLM to evaluate document relevance
type DocumentRelevanceEvaluator struct {
	llmProvider llm.Provider
}

// NewDocumentRelevanceEvaluator creates a new relevance evaluator
func NewDocumentRelevanceEvaluator(llmProvider llm.Provider) *DocumentRelevanceEvaluator {
	return &DocumentRelevanceEvaluator{
		llmProvider: llmProvider,
	}
}

// EvaluateRelevance determines if a document is relevant to a search query
// based on the file path and name
func (e *DocumentRelevanceEvaluator) EvaluateRelevance(ctx context.Context, searchQuery string, filePath string) (bool, error) {
	// Extract filename and directory info
	filename := filepath.Base(filePath)
	dirname := filepath.Dir(filePath)
	
	// Create evaluation prompt using template
	prompt := prompts.RenderDocumentRelevancePrompt(prompts.PromptData{
		SearchQuery: searchQuery,
		FileName:    filename,
		DirName:     dirname,
		FilePath:    filePath,
	})

	// Create completion request
	request := &llm.CompletionRequest{
		Model: "claude-3-haiku-20240307", // Use fast model for this evaluation
		Messages: []llm.Message{
			{
				Role:    "user", 
				Content: prompt,
			},
		},
		MaxTokens:   10,
		Temperature: 0.1, // Low temperature for consistent results
	}

	// Get LLM response
	response, err := e.llmProvider.Complete(ctx, request)
	if err != nil {
		return false, fmt.Errorf("failed to evaluate relevance: %v", err)
	}

	// Parse response
	result := strings.TrimSpace(strings.ToUpper(response.Content))
	return result == "RELEVANT", nil
}

// BatchEvaluateRelevance evaluates multiple documents for relevance
func (e *DocumentRelevanceEvaluator) BatchEvaluateRelevance(ctx context.Context, searchQuery string, filePaths []string) ([]string, error) {
	var relevantPaths []string
	
	for _, filePath := range filePaths {
		relevant, err := e.EvaluateRelevance(ctx, searchQuery, filePath)
		if err != nil {
			// Log error but continue with other files
			continue
		}
		
		if relevant {
			relevantPaths = append(relevantPaths, filePath)
		}
	}
	
	return relevantPaths, nil
}