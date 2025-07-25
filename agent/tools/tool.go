package tools

import (
	"context"
	"encoding/json"
	"time"

	"github.com/santiagocorredoira/agent/agent/llm"
)

// Tool defines the interface that all tools must implement
type Tool interface {
	// GetName returns the unique name of the tool
	GetName() string
	
	// GetDescription returns a human-readable description of what the tool does
	GetDescription() string
	
	// GetParameterSchema returns the JSON schema for the tool's parameters
	GetParameterSchema() *ParameterSchema
	
	// Execute runs the tool with the given parameters
	Execute(ctx context.Context, params map[string]interface{}) (*ToolResult, error)
	
	// IsAvailable checks if the tool is currently available for use
	IsAvailable(ctx context.Context) bool
	
	// GetCategory returns the category this tool belongs to
	GetCategory() ToolCategory
	
	// RequiresConfirmation returns true if this tool requires user confirmation before execution
	RequiresConfirmation() bool
	
	// GetEstimatedCost returns an estimated cost/complexity for this operation (0-100)
	GetEstimatedCost() int
	
	// GetFunctionDefinition returns the LLM function definition for this tool
	GetFunctionDefinition() llm.FunctionDefinition
}

// ParameterSchema defines the expected parameters for a tool
type ParameterSchema struct {
	Type        string                        `json:"type"`
	Properties  map[string]PropertySchema     `json:"properties"`
	Required    []string                      `json:"required"`
	Description string                        `json:"description,omitempty"`
}

// ToJSONSchema converts ParameterSchema to the JSON Schema format expected by LLMs
func (ps *ParameterSchema) ToJSONSchema() map[string]interface{} {
	schema := map[string]interface{}{
		"type": ps.Type,
	}
	
	if ps.Properties != nil {
		properties := make(map[string]interface{})
		for name, prop := range ps.Properties {
			propSchema := map[string]interface{}{
				"type": prop.Type,
			}
			if prop.Description != "" {
				propSchema["description"] = prop.Description
			}
			if prop.Enum != nil {
				propSchema["enum"] = prop.Enum
			}
			if prop.Default != nil {
				propSchema["default"] = prop.Default
			}
			if prop.Format != "" {
				propSchema["format"] = prop.Format
			}
			if prop.Minimum != nil {
				propSchema["minimum"] = *prop.Minimum
			}
			if prop.Maximum != nil {
				propSchema["maximum"] = *prop.Maximum
			}
			if prop.Items != nil {
				itemSchema := map[string]interface{}{
					"type": prop.Items.Type,
				}
				if prop.Items.Description != "" {
					itemSchema["description"] = prop.Items.Description
				}
				propSchema["items"] = itemSchema
			}
			properties[name] = propSchema
		}
		schema["properties"] = properties
	}
	
	if len(ps.Required) > 0 {
		schema["required"] = ps.Required
	}
	
	return schema
}

// DefaultGetFunctionDefinition provides a default implementation for GetFunctionDefinition
func DefaultGetFunctionDefinition(tool Tool) llm.FunctionDefinition {
	return llm.FunctionDefinition{
		Name:        tool.GetName(),
		Description: tool.GetDescription(),
		Parameters:  tool.GetParameterSchema().ToJSONSchema(),
	}
}

// PropertySchema defines the schema for a single parameter property
type PropertySchema struct {
	Type        string           `json:"type"`
	Description string           `json:"description,omitempty"`
	Enum        []interface{}    `json:"enum,omitempty"`
	Default     interface{}      `json:"default,omitempty"`
	Format      string           `json:"format,omitempty"`
	MinLength   *int             `json:"minLength,omitempty"`
	MaxLength   *int             `json:"maxLength,omitempty"`
	Minimum     *float64         `json:"minimum,omitempty"`
	Maximum     *float64         `json:"maximum,omitempty"`
	Items       *PropertySchema  `json:"items,omitempty"`
}

// ToolResult represents the result of executing a tool
type ToolResult struct {
	Success     bool                   `json:"success"`
	Data        interface{}            `json:"data,omitempty"`
	Error       string                 `json:"error,omitempty"`
	Message     string                 `json:"message,omitempty"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
	ExecutionID string                 `json:"execution_id"`
	Timestamp   time.Time              `json:"timestamp"`
	Duration    time.Duration          `json:"duration"`
}

// ToolCategory represents different categories of tools
type ToolCategory string

const (
	CategoryAPI         ToolCategory = "api"
	CategoryFile        ToolCategory = "file"
	CategoryWeb         ToolCategory = "web"
	CategoryData        ToolCategory = "data"
	CategorySystem      ToolCategory = "system"
	CategoryCalculation ToolCategory = "calculation"
	CategoryText        ToolCategory = "text"
	CategoryImage       ToolCategory = "image"
	CategoryCustom      ToolCategory = "custom"
)

// ToolInfo provides metadata about a tool without exposing the full interface
type ToolInfo struct {
	Name                string       `json:"name"`
	Description         string       `json:"description"`
	Category            ToolCategory `json:"category"`
	Available           bool         `json:"available"`
	RequiresConfirmation bool        `json:"requires_confirmation"`
	EstimatedCost       int          `json:"estimated_cost"`
	ParameterSchema     *ParameterSchema `json:"parameter_schema,omitempty"`
}

// ToolExecution represents a tool execution request
type ToolExecution struct {
	ToolName    string                 `json:"tool_name"`
	Parameters  map[string]interface{} `json:"parameters"`
	RequestID   string                 `json:"request_id,omitempty"`
	UserID      string                 `json:"user_id,omitempty"`
	SessionID   string                 `json:"session_id,omitempty"`
	Confirmed   bool                   `json:"confirmed"`
	Timestamp   time.Time              `json:"timestamp"`
}

// ToolExecutionHistory represents a historical tool execution
type ToolExecutionHistory struct {
	Execution ToolExecution `json:"execution"`
	Result    ToolResult    `json:"result"`
	Success   bool          `json:"success"`
}

// BaseTool provides common functionality for all tools
type BaseTool struct {
	name                string
	description         string
	category            ToolCategory
	requiresConfirmation bool
	estimatedCost       int
	parameterSchema     *ParameterSchema
}

// NewBaseTool creates a new base tool with common properties
func NewBaseTool(name, description string, category ToolCategory, requiresConfirmation bool, estimatedCost int) *BaseTool {
	return &BaseTool{
		name:                name,
		description:         description,
		category:            category,
		requiresConfirmation: requiresConfirmation,
		estimatedCost:       estimatedCost,
	}
}

// GetName returns the tool name
func (bt *BaseTool) GetName() string {
	return bt.name
}

// GetDescription returns the tool description
func (bt *BaseTool) GetDescription() string {
	return bt.description
}

// GetCategory returns the tool category
func (bt *BaseTool) GetCategory() ToolCategory {
	return bt.category
}

// RequiresConfirmation returns whether the tool requires confirmation
func (bt *BaseTool) RequiresConfirmation() bool {
	return bt.requiresConfirmation
}

// GetEstimatedCost returns the estimated cost
func (bt *BaseTool) GetEstimatedCost() int {
	return bt.estimatedCost
}

// SetParameterSchema sets the parameter schema for the tool
func (bt *BaseTool) SetParameterSchema(schema *ParameterSchema) {
	bt.parameterSchema = schema
}

// GetParameterSchema returns the parameter schema
func (bt *BaseTool) GetParameterSchema() *ParameterSchema {
	return bt.parameterSchema
}

// ValidateParameters validates parameters against the schema
func (bt *BaseTool) ValidateParameters(params map[string]interface{}) error {
	if bt.parameterSchema == nil {
		return nil // No schema means no validation required
	}

	// Check required parameters
	for _, required := range bt.parameterSchema.Required {
		if _, exists := params[required]; !exists {
			return &ToolError{
				Type:    "validation_error",
				Message: "missing required parameter: " + required,
				Code:    "MISSING_REQUIRED_PARAM",
			}
		}
	}

	// Validate each parameter against its schema
	for paramName, paramValue := range params {
		propSchema, exists := bt.parameterSchema.Properties[paramName]
		if !exists {
			return &ToolError{
				Type:    "validation_error", 
				Message: "unknown parameter: " + paramName,
				Code:    "UNKNOWN_PARAM",
			}
		}

		if err := validateParameterValue(paramValue, propSchema); err != nil {
			return &ToolError{
				Type:    "validation_error",
				Message: "invalid value for parameter " + paramName + ": " + err.Error(),
				Code:    "INVALID_PARAM_VALUE",
			}
		}
	}

	return nil
}

// CreateSuccessResult creates a successful tool result
func (bt *BaseTool) CreateSuccessResult(data interface{}, message string) *ToolResult {
	return &ToolResult{
		Success:     true,
		Data:        data,
		Message:     message,
		ExecutionID: generateExecutionID(),
		Timestamp:   time.Now(),
	}
}

// CreateErrorResult creates an error tool result
func (bt *BaseTool) CreateErrorResult(err error, message string) *ToolResult {
	return &ToolResult{
		Success:     false,
		Error:       err.Error(),
		Message:     message,
		ExecutionID: generateExecutionID(),
		Timestamp:   time.Now(),
	}
}

// ToolError represents tool-specific errors
type ToolError struct {
	Type    string `json:"type"`
	Message string `json:"message"`
	Code    string `json:"code"`
	Details interface{} `json:"details,omitempty"`
}

func (e *ToolError) Error() string {
	return e.Message
}

// Helper functions

func validateParameterValue(value interface{}, schema PropertySchema) error {
	// Basic type validation - can be extended
	switch schema.Type {
	case "string":
		if _, ok := value.(string); !ok {
			return &ToolError{Type: "type_error", Message: "expected string", Code: "TYPE_MISMATCH"}
		}
	case "number":
		switch value.(type) {
		case int, int64, float64, float32:
			// Valid number types
		default:
			return &ToolError{Type: "type_error", Message: "expected number", Code: "TYPE_MISMATCH"}
		}
	case "boolean":
		if _, ok := value.(bool); !ok {
			return &ToolError{Type: "type_error", Message: "expected boolean", Code: "TYPE_MISMATCH"}
		}
	case "array":
		if _, ok := value.([]interface{}); !ok {
			return &ToolError{Type: "type_error", Message: "expected array", Code: "TYPE_MISMATCH"}
		}
	case "object":
		if _, ok := value.(map[string]interface{}); !ok {
			return &ToolError{Type: "type_error", Message: "expected object", Code: "TYPE_MISMATCH"}
		}
	}

	return nil
}

func generateExecutionID() string {
	// Simple execution ID generation - could use UUID in production
	return "exec_" + time.Now().Format("20060102_150405") + "_" + randomString(6)
}

func randomString(length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyz0123456789"
	result := make([]byte, length)
	for i := range result {
		result[i] = charset[time.Now().UnixNano()%int64(len(charset))]
	}
	return string(result)
}

// GetToolInfo extracts ToolInfo from a Tool interface
func GetToolInfo(tool Tool, ctx context.Context) ToolInfo {
	return ToolInfo{
		Name:                tool.GetName(),
		Description:         tool.GetDescription(),
		Category:            tool.GetCategory(),
		Available:           tool.IsAvailable(ctx),
		RequiresConfirmation: tool.RequiresConfirmation(),
		EstimatedCost:       tool.GetEstimatedCost(),
		ParameterSchema:     tool.GetParameterSchema(),
	}
}

// MarshalToolResult marshals a ToolResult to JSON
func MarshalToolResult(result *ToolResult) ([]byte, error) {
	return json.MarshalIndent(result, "", "  ")
}

// UnmarshalToolResult unmarshals JSON to a ToolResult
func UnmarshalToolResult(data []byte) (*ToolResult, error) {
	var result ToolResult
	err := json.Unmarshal(data, &result)
	return &result, err
}