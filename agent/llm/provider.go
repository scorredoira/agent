package llm

import (
	"context"
	"time"
)

// Message representa un mensaje en la conversación
type Message struct {
	Role       string     `json:"role"`                 // "user", "assistant", "system", "tool"
	Content    string     `json:"content"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"` // For assistant messages with tool calls
	ToolCallID string     `json:"tool_call_id,omitempty"` // For tool response messages
}

// CompletionRequest representa una solicitud de completado
type CompletionRequest struct {
	Messages    []Message          `json:"messages"`
	Model       string             `json:"model,omitempty"`
	MaxTokens   int                `json:"max_tokens,omitempty"`
	Temperature float64            `json:"temperature,omitempty"`
	Stream      bool               `json:"stream,omitempty"`
	Tools       []FunctionTool     `json:"tools,omitempty"`       // Function calling tools
	ToolChoice  string             `json:"tool_choice,omitempty"` // "auto", "none", or specific tool
}

// CompletionResponse representa una respuesta de completado
type CompletionResponse struct {
	Content      string        `json:"content"`
	Model        string        `json:"model"`
	Usage        TokenUsage    `json:"usage"`
	ResponseTime time.Duration `json:"response_time"`
	ToolCalls    []ToolCall    `json:"tool_calls,omitempty"` // Function calls requested by LLM
}

// TokenUsage representa el uso de tokens
type TokenUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// StreamChunk representa un chunk de respuesta en streaming
type StreamChunk struct {
	Content string `json:"content"`
	Done    bool   `json:"done"`
}

// Provider define la interfaz que deben implementar todos los proveedores LLM
type Provider interface {
	// GetName devuelve el nombre del proveedor
	GetName() string
	
	// IsAvailable verifica si el proveedor está disponible y configurado
	IsAvailable(ctx context.Context) bool
	
	// Complete envía una solicitud de completado y devuelve la respuesta
	Complete(ctx context.Context, req *CompletionRequest) (*CompletionResponse, error)
	
	// Stream envía una solicitud de completado y devuelve un canal de chunks
	Stream(ctx context.Context, req *CompletionRequest) (<-chan StreamChunk, error)
	
	// GetModels devuelve la lista de modelos disponibles para este proveedor
	GetModels() []string
	
	// GetDefaultModel devuelve el modelo por defecto para este proveedor
	GetDefaultModel() string
	
	// ValidateConfig valida la configuración del proveedor
	ValidateConfig() error
	
	// SupportsFunctionCalling indica si el proveedor soporta function calling
	SupportsFunctionCalling() bool
}

// Config representa la configuración de un proveedor LLM
type Config struct {
	APIKey      string            `json:"api_key"`
	BaseURL     string            `json:"base_url,omitempty"`
	Model       string            `json:"model,omitempty"`
	MaxTokens   int               `json:"max_tokens,omitempty"`
	Temperature float64           `json:"temperature,omitempty"`
	Timeout     time.Duration     `json:"timeout,omitempty"`
	Extra       map[string]string `json:"extra,omitempty"`
}

// ProviderType representa los tipos de proveedores disponibles
type ProviderType string

const (
	ProviderAnthropic ProviderType = "anthropic"
	ProviderOpenAI    ProviderType = "openai"
	ProviderGemini    ProviderType = "gemini"
)

// ProviderError representa un error específico de un proveedor
type ProviderError struct {
	Provider string
	Type     string
	Message  string
	Err      error
}

func (e *ProviderError) Error() string {
	if e.Err != nil {
		return e.Provider + " " + e.Type + ": " + e.Message + " (" + e.Err.Error() + ")"
	}
	return e.Provider + " " + e.Type + ": " + e.Message
}

// ErrorType constantes para tipos de errores
const (
	ErrorTypeAuth        = "auth_error"
	ErrorTypeRateLimit   = "rate_limit"
	ErrorTypeQuotaExceed = "quota_exceeded"
	ErrorTypeNetwork     = "network_error"
	ErrorTypeInvalidReq  = "invalid_request"
	ErrorTypeServerError = "server_error"
)

// Function Calling Types

// FunctionTool represents a tool/function available to the LLM
type FunctionTool struct {
	Type     string            `json:"type"`     // Always "function"
	Function FunctionDefinition `json:"function"`
}

// FunctionDefinition defines a function that can be called by the LLM
type FunctionDefinition struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Parameters  map[string]interface{} `json:"parameters"` // JSON Schema
}

// ToolCall represents a function call requested by the LLM
type ToolCall struct {
	ID       string       `json:"id"`       // Unique identifier for this call
	Type     string       `json:"type"`     // Always "function"
	Function FunctionCall `json:"function"`
}

// FunctionCall contains the details of a function call
type FunctionCall struct {
	Name      string `json:"name"`      // Function name to call
	Arguments string `json:"arguments"` // JSON string of arguments
}

// ToolResult represents the result of executing a tool
type ToolResult struct {
	ToolCallID string `json:"tool_call_id"`
	Content    string `json:"content"`
	IsError    bool   `json:"is_error,omitempty"`
}