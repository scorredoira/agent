package tools

import (
	"context"

	"github.com/santiagocorredoira/agent/agent/llm"
)

// LegacyTool represents the old Tool interface without GetFunctionDefinition
type LegacyTool interface {
	GetName() string
	GetDescription() string
	GetParameterSchema() *ParameterSchema
	Execute(ctx context.Context, params map[string]interface{}) (*ToolResult, error)
	IsAvailable(ctx context.Context) bool
	GetCategory() ToolCategory
	RequiresConfirmation() bool
	GetEstimatedCost() int
}

// ToolWrapper wraps legacy tools to implement the new Tool interface
type ToolWrapper struct {
	LegacyTool
}

// WrapLegacyTool wraps a legacy tool to implement the new interface
func WrapLegacyTool(legacyTool LegacyTool) Tool {
	return &ToolWrapper{LegacyTool: legacyTool}
}

// GetFunctionDefinition implements the new method for legacy tools
func (w *ToolWrapper) GetFunctionDefinition() llm.FunctionDefinition {
	return DefaultGetFunctionDefinition(w)
}

// Ensure ToolWrapper implements Tool interface
var _ Tool = (*ToolWrapper)(nil)