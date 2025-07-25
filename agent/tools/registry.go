package tools

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

// ToolRegistry manages the collection of available tools
type ToolRegistry struct {
	tools    map[string]Tool
	history  []ToolExecutionHistory
	mu       sync.RWMutex
	maxHistory int
}

// NewToolRegistry creates a new tool registry
func NewToolRegistry() *ToolRegistry {
	return &ToolRegistry{
		tools:      make(map[string]Tool),
		history:    make([]ToolExecutionHistory, 0),
		maxHistory: 1000, // Keep last 1000 executions
	}
}

// RegisterTool registers a new tool in the registry
func (tr *ToolRegistry) RegisterTool(tool Tool) error {
	if tool == nil {
		return fmt.Errorf("cannot register nil tool")
	}

	name := tool.GetName()
	if name == "" {
		return fmt.Errorf("tool name cannot be empty")
	}

	tr.mu.Lock()
	defer tr.mu.Unlock()

	if _, exists := tr.tools[name]; exists {
		return fmt.Errorf("tool '%s' is already registered", name)
	}

	tr.tools[name] = tool
	return nil
}

// UnregisterTool removes a tool from the registry
func (tr *ToolRegistry) UnregisterTool(name string) bool {
	tr.mu.Lock()
	defer tr.mu.Unlock()

	if _, exists := tr.tools[name]; exists {
		delete(tr.tools, name)
		return true
	}
	return false
}

// GetTool retrieves a tool by name
func (tr *ToolRegistry) GetTool(name string) (Tool, bool) {
	tr.mu.RLock()
	defer tr.mu.RUnlock()

	tool, exists := tr.tools[name]
	return tool, exists
}

// ListTools returns a list of all registered tools
func (tr *ToolRegistry) ListTools(ctx context.Context) []ToolInfo {
	tr.mu.RLock()
	defer tr.mu.RUnlock()

	var tools []ToolInfo
	for _, tool := range tr.tools {
		tools = append(tools, GetToolInfo(tool, ctx))
	}

	// Sort by category, then by name
	sort.Slice(tools, func(i, j int) bool {
		if tools[i].Category != tools[j].Category {
			return tools[i].Category < tools[j].Category
		}
		return tools[i].Name < tools[j].Name
	})

	return tools
}

// ListAvailableTools returns only tools that are currently available
func (tr *ToolRegistry) ListAvailableTools(ctx context.Context) []ToolInfo {
	tools := tr.ListTools(ctx)
	var available []ToolInfo

	for _, tool := range tools {
		if tool.Available {
			available = append(available, tool)
		}
	}

	return available
}

// GetToolsByCategory returns tools filtered by category
func (tr *ToolRegistry) GetToolsByCategory(ctx context.Context, category ToolCategory) []ToolInfo {
	tools := tr.ListTools(ctx)
	var filtered []ToolInfo

	for _, tool := range tools {
		if tool.Category == category {
			filtered = append(filtered, tool)
		}
	}

	return filtered
}

// SearchTools searches for tools by name or description
func (tr *ToolRegistry) SearchTools(ctx context.Context, query string) []ToolInfo {
	tools := tr.ListTools(ctx)
	var results []ToolInfo
	queryLower := strings.ToLower(query)

	for _, tool := range tools {
		nameMatch := strings.Contains(strings.ToLower(tool.Name), queryLower)
		descMatch := strings.Contains(strings.ToLower(tool.Description), queryLower)

		if nameMatch || descMatch {
			results = append(results, tool)
		}
	}

	return results
}

// ExecuteTool executes a tool with the given parameters
func (tr *ToolRegistry) ExecuteTool(ctx context.Context, execution ToolExecution) (*ToolResult, error) {
	tool, exists := tr.GetTool(execution.ToolName)
	if !exists {
		return nil, fmt.Errorf("tool '%s' not found", execution.ToolName)
	}

	// Check if tool is available
	if !tool.IsAvailable(ctx) {
		return &ToolResult{
			Success:     false,
			Error:       "tool is not available",
			Message:     fmt.Sprintf("Tool '%s' is currently unavailable", execution.ToolName),
			ExecutionID: generateExecutionID(),
			Timestamp:   time.Now(),
		}, nil
	}

	// Check confirmation requirement
	if tool.RequiresConfirmation() && !execution.Confirmed {
		return &ToolResult{
			Success:     false,
			Error:       "confirmation required",
			Message:     fmt.Sprintf("Tool '%s' requires user confirmation before execution", execution.ToolName),
			ExecutionID: generateExecutionID(),
			Timestamp:   time.Now(),
			Metadata: map[string]interface{}{
				"requires_confirmation": true,
				"estimated_cost":        tool.GetEstimatedCost(),
			},
		}, nil
	}

	// Validate parameters if the tool has a ValidateParameters method
	if validator, ok := tool.(interface{ ValidateParameters(map[string]interface{}) error }); ok {
		if err := validator.ValidateParameters(execution.Parameters); err != nil {
			return &ToolResult{
				Success:     false,
				Error:       err.Error(),
				Message:     fmt.Sprintf("Parameter validation failed for tool '%s'", execution.ToolName),
				ExecutionID: generateExecutionID(),
				Timestamp:   time.Now(),
			}, nil
		}
	}

	// Execute the tool
	start := time.Now()
	result, err := tool.Execute(ctx, execution.Parameters)
	if err != nil {
		result = &ToolResult{
			Success:     false,
			Error:       err.Error(),
			Message:     fmt.Sprintf("Tool '%s' execution failed", execution.ToolName),
			ExecutionID: generateExecutionID(),
			Timestamp:   time.Now(),
			Duration:    time.Since(start),
		}
	} else {
		result.Duration = time.Since(start)
	}

	// Record execution in history
	tr.recordExecution(execution, *result)

	return result, nil
}

// GetExecutionHistory returns the execution history
func (tr *ToolRegistry) GetExecutionHistory(limit int) []ToolExecutionHistory {
	tr.mu.RLock()
	defer tr.mu.RUnlock()

	if limit <= 0 || limit > len(tr.history) {
		limit = len(tr.history)
	}

	// Return the most recent executions
	start := len(tr.history) - limit
	return tr.history[start:]
}

// GetToolUsageStats returns usage statistics for tools
func (tr *ToolRegistry) GetToolUsageStats() map[string]ToolUsageStats {
	tr.mu.RLock()
	defer tr.mu.RUnlock()

	stats := make(map[string]ToolUsageStats)

	for _, execution := range tr.history {
		toolName := execution.Execution.ToolName
		stat := stats[toolName]
		
		stat.ToolName = toolName
		stat.TotalExecutions++
		stat.TotalDuration += execution.Result.Duration

		if execution.Result.Success {
			stat.SuccessfulExecutions++
		} else {
			stat.FailedExecutions++
		}

		if execution.Result.Duration > stat.MaxDuration {
			stat.MaxDuration = execution.Result.Duration
		}

		if stat.MinDuration == 0 || execution.Result.Duration < stat.MinDuration {
			stat.MinDuration = execution.Result.Duration
		}

		stat.LastUsed = execution.Result.Timestamp
		stats[toolName] = stat
	}

	// Calculate averages
	for toolName, stat := range stats {
		if stat.TotalExecutions > 0 {
			stat.AverageDuration = stat.TotalDuration / time.Duration(stat.TotalExecutions)
			stat.SuccessRate = float64(stat.SuccessfulExecutions) / float64(stat.TotalExecutions)
		}
		stats[toolName] = stat
	}

	return stats
}

// GetToolStats returns overall registry statistics
func (tr *ToolRegistry) GetToolStats() RegistryStats {
	tr.mu.RLock()
	defer tr.mu.RUnlock()

	stats := RegistryStats{
		TotalTools:      len(tr.tools),
		TotalExecutions: len(tr.history),
		Categories:      make(map[ToolCategory]int),
	}

	ctx := context.Background()
	for _, tool := range tr.tools {
		category := tool.GetCategory()
		stats.Categories[category]++

		if tool.IsAvailable(ctx) {
			stats.AvailableTools++
		}
	}

	if len(tr.history) > 0 {
		successCount := 0
		for _, execution := range tr.history {
			if execution.Result.Success {
				successCount++
			}
		}
		stats.OverallSuccessRate = float64(successCount) / float64(len(tr.history))
		stats.LastExecution = tr.history[len(tr.history)-1].Result.Timestamp
	}

	return stats
}

// ClearHistory clears the execution history
func (tr *ToolRegistry) ClearHistory() {
	tr.mu.Lock()
	defer tr.mu.Unlock()
	tr.history = make([]ToolExecutionHistory, 0)
}

// Private methods

func (tr *ToolRegistry) recordExecution(execution ToolExecution, result ToolResult) {
	tr.mu.Lock()
	defer tr.mu.Unlock()

	history := ToolExecutionHistory{
		Execution: execution,
		Result:    result,
		Success:   result.Success,
	}

	tr.history = append(tr.history, history)

	// Maintain max history limit
	if len(tr.history) > tr.maxHistory {
		tr.history = tr.history[len(tr.history)-tr.maxHistory:]
	}
}

// Supporting types

// ToolUsageStats represents usage statistics for a specific tool
type ToolUsageStats struct {
	ToolName             string        `json:"tool_name"`
	TotalExecutions      int           `json:"total_executions"`
	SuccessfulExecutions int           `json:"successful_executions"`
	FailedExecutions     int           `json:"failed_executions"`
	SuccessRate          float64       `json:"success_rate"`
	TotalDuration        time.Duration `json:"total_duration"`
	AverageDuration      time.Duration `json:"average_duration"`
	MinDuration          time.Duration `json:"min_duration"`
	MaxDuration          time.Duration `json:"max_duration"`
	LastUsed             time.Time     `json:"last_used"`
}

// RegistryStats represents overall registry statistics
type RegistryStats struct {
	TotalTools         int                      `json:"total_tools"`
	AvailableTools     int                      `json:"available_tools"`
	Categories         map[ToolCategory]int     `json:"categories"`
	TotalExecutions    int                      `json:"total_executions"`
	OverallSuccessRate float64                  `json:"overall_success_rate"`
	LastExecution      time.Time                `json:"last_execution"`
}

// ToolFilter represents filtering criteria for tools
type ToolFilter struct {
	Category            *ToolCategory `json:"category,omitempty"`
	Available           *bool         `json:"available,omitempty"`
	RequiresConfirmation *bool        `json:"requires_confirmation,omitempty"`
	MaxCost             *int          `json:"max_cost,omitempty"`
	MinCost             *int          `json:"min_cost,omitempty"`
}

// FilterTools applies filters to the tool list
func (tr *ToolRegistry) FilterTools(ctx context.Context, filter ToolFilter) []ToolInfo {
	tools := tr.ListTools(ctx)
	var filtered []ToolInfo

	for _, tool := range tools {
		if filter.Category != nil && tool.Category != *filter.Category {
			continue
		}
		if filter.Available != nil && tool.Available != *filter.Available {
			continue
		}
		if filter.RequiresConfirmation != nil && tool.RequiresConfirmation != *filter.RequiresConfirmation {
			continue
		}
		if filter.MaxCost != nil && tool.EstimatedCost > *filter.MaxCost {
			continue
		}
		if filter.MinCost != nil && tool.EstimatedCost < *filter.MinCost {
			continue
		}

		filtered = append(filtered, tool)
	}

	return filtered
}