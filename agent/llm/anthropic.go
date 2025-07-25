package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// AnthropicProvider implementa el proveedor para Claude de Anthropic
type AnthropicProvider struct {
	config     *Config
	httpClient *http.Client
}

// NewAnthropicProvider crea una nueva instancia del proveedor Anthropic
func NewAnthropicProvider(config *Config) *AnthropicProvider {
	if config.BaseURL == "" {
		config.BaseURL = "https://api.anthropic.com"
	}
	if config.Model == "" {
		config.Model = "claude-3-5-sonnet-20241022"
	}
	if config.MaxTokens == 0 {
		config.MaxTokens = 4096
	}
	if config.Temperature == 0 {
		config.Temperature = 0.7
	}
	if config.Timeout == 0 {
		config.Timeout = 60 * time.Second
	}

	return &AnthropicProvider{
		config: config,
		httpClient: &http.Client{
			Timeout: config.Timeout,
		},
	}
}

// GetName devuelve el nombre del proveedor
func (p *AnthropicProvider) GetName() string {
	return "anthropic"
}

// IsAvailable verifica si el proveedor está disponible
func (p *AnthropicProvider) IsAvailable(ctx context.Context) bool {
	if p.config.APIKey == "" {
		return false
	}

	// Hacer una prueba básica con un mensaje simple
	testReq := &CompletionRequest{
		Messages: []Message{
			{Role: "user", Content: "Hello"},
		},
		Model:     p.config.Model,
		MaxTokens: 10,
	}

	_, err := p.Complete(ctx, testReq)
	return err == nil
}

// Complete envía una solicitud de completado
func (p *AnthropicProvider) Complete(ctx context.Context, req *CompletionRequest) (*CompletionResponse, error) {
	start := time.Now()

	// Preparar la solicitud para la API de Anthropic
	anthropicReq := p.buildAnthropicRequest(req)
	
	jsonData, err := json.Marshal(anthropicReq)
	if err != nil {
		return nil, &ProviderError{
			Provider: p.GetName(),
			Type:     ErrorTypeInvalidReq,
			Message:  "failed to marshal request",
			Err:      err,
		}
	}

	// Debug: uncomment for debugging
	// if len(req.Tools) > 0 {
	//	fmt.Printf("🐛 Anthropic request with %d tools:\n%s\n", len(req.Tools), string(jsonData))
	// }

	// Crear la solicitud HTTP
	httpReq, err := http.NewRequestWithContext(ctx, "POST", p.config.BaseURL+"/v1/messages", bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, &ProviderError{
			Provider: p.GetName(),
			Type:     ErrorTypeNetwork,
			Message:  "failed to create HTTP request",
			Err:      err,
		}
	}

	// Configurar headers
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", p.config.APIKey)
	httpReq.Header.Set("anthropic-version", "2023-06-01")

	// Enviar la solicitud
	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return nil, &ProviderError{
			Provider: p.GetName(),
			Type:     ErrorTypeNetwork,
			Message:  "failed to send request",
			Err:      err,
		}
	}
	defer resp.Body.Close()

	// Leer la respuesta
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, &ProviderError{
			Provider: p.GetName(),
			Type:     ErrorTypeNetwork,
			Message:  "failed to read response",
			Err:      err,
		}
	}

	// Manejar errores HTTP
	if resp.StatusCode != http.StatusOK {
		return nil, p.handleHTTPError(resp.StatusCode, body)
	}

	// Debug: uncomment for debugging
	// if len(req.Tools) > 0 {
	//	fmt.Printf("🐛 Anthropic raw response:\n%s\n", string(body))
	// }

	// Parsear la respuesta
	var anthropicResp AnthropicResponse
	if err := json.Unmarshal(body, &anthropicResp); err != nil {
		return nil, &ProviderError{
			Provider: p.GetName(),
			Type:     ErrorTypeServerError,
			Message:  "failed to parse response",
			Err:      err,
		}
	}

	// Convertir a nuestro formato estándar
	content := ""
	var toolCalls []ToolCall

	// Debug: uncomment for debugging
	// fmt.Printf("🐛 Processing %d content blocks\n", len(anthropicResp.Content))
	for _, contentBlock := range anthropicResp.Content {
		// fmt.Printf("🐛 Block %d: Type=%s, ID=%s, Name=%s\n", i, contentBlock.Type, contentBlock.ID, contentBlock.Name)
		switch contentBlock.Type {
		case "text":
			content += contentBlock.Text
		case "tool_use":
			// Convertir input a JSON string
			inputJSON, err := json.Marshal(contentBlock.Input)
			if err != nil {
				inputJSON = []byte("{}")
			}

			toolCalls = append(toolCalls, ToolCall{
				ID:   contentBlock.ID,
				Type: "function",
				Function: FunctionCall{
					Name:      contentBlock.Name,
					Arguments: string(inputJSON),
				},
			})
		}
	}

	response := &CompletionResponse{
		Content: content,
		Model:   anthropicResp.Model,
		Usage: TokenUsage{
			PromptTokens:     anthropicResp.Usage.InputTokens,
			CompletionTokens: anthropicResp.Usage.OutputTokens,
			TotalTokens:      anthropicResp.Usage.InputTokens + anthropicResp.Usage.OutputTokens,
		},
		ResponseTime: time.Since(start),
	}

	// Agregar tool calls si existen
	if len(toolCalls) > 0 {
		response.ToolCalls = toolCalls
	}

	return response, nil
}

// Stream implementa streaming para Anthropic
func (p *AnthropicProvider) Stream(ctx context.Context, req *CompletionRequest) (<-chan StreamChunk, error) {
	ch := make(chan StreamChunk, 10)
	
	// Por ahora, implementamos streaming simulado usando Complete
	// En una implementación completa, usaríamos Server-Sent Events
	go func() {
		defer close(ch)
		
		resp, err := p.Complete(ctx, req)
		if err != nil {
			ch <- StreamChunk{Content: fmt.Sprintf("Error: %v", err), Done: true}
			return
		}
		
		// Simular streaming dividiendo la respuesta en chunks
		words := strings.Fields(resp.Content)
		for _, word := range words {
			select {
			case <-ctx.Done():
				return
			case ch <- StreamChunk{Content: word + " ", Done: false}:
				time.Sleep(50 * time.Millisecond) // Simular latencia
			}
		}
		
		ch <- StreamChunk{Content: "", Done: true}
	}()
	
	return ch, nil
}

// GetModels devuelve los modelos disponibles
func (p *AnthropicProvider) GetModels() []string {
	return []string{
		"claude-3-5-sonnet-20241022",
		"claude-3-opus-20240229",
		"claude-3-sonnet-20240229",
		"claude-3-haiku-20240307",
	}
}

// GetDefaultModel devuelve el modelo por defecto
func (p *AnthropicProvider) GetDefaultModel() string {
	return "claude-3-5-sonnet-20241022"
}

// ValidateConfig valida la configuración
func (p *AnthropicProvider) ValidateConfig() error {
	if p.config.APIKey == "" {
		return fmt.Errorf("API key is required for Anthropic provider")
	}
	return nil
}

// SupportsFunctionCalling indica si Anthropic soporta function calling
func (p *AnthropicProvider) SupportsFunctionCalling() bool {
	return true
}

// buildAnthropicRequest convierte nuestra solicitud al formato de Anthropic
func (p *AnthropicProvider) buildAnthropicRequest(req *CompletionRequest) *AnthropicRequest {
	anthropicReq := &AnthropicRequest{
		Model:       req.Model,
		MaxTokens:   req.MaxTokens,
		Temperature: req.Temperature,
		Messages:    make([]AnthropicMessage, 0, len(req.Messages)),
	}

	// Si no se especifica modelo, usar el por defecto
	if anthropicReq.Model == "" {
		anthropicReq.Model = p.config.Model
	}
	if anthropicReq.MaxTokens == 0 {
		anthropicReq.MaxTokens = p.config.MaxTokens
	}
	if anthropicReq.Temperature == 0 {
		anthropicReq.Temperature = p.config.Temperature
	}

	// Convertir herramientas si están disponibles
	if len(req.Tools) > 0 {
		anthropicReq.Tools = make([]AnthropicTool, len(req.Tools))
		for i, tool := range req.Tools {
			inputSchema := AnthropicToolInputSchema{
				Type: "object",
			}
			
			// Extract properties and required fields from the JSON schema
			if props, ok := tool.Function.Parameters["properties"].(map[string]interface{}); ok {
				inputSchema.Properties = props
			}
			if required, ok := tool.Function.Parameters["required"].([]interface{}); ok {
				requiredStrings := make([]string, len(required))
				for j, req := range required {
					if str, ok := req.(string); ok {
						requiredStrings[j] = str
					}
				}
				inputSchema.Required = requiredStrings
			}
			
			anthropicReq.Tools[i] = AnthropicTool{
				Name:        tool.Function.Name,
				Description: tool.Function.Description,
				InputSchema: inputSchema,
			}
		}
	}

	// Convertir mensajes al formato de Anthropic
	for _, msg := range req.Messages {
		if msg.Role == "system" {
			// Anthropic maneja system messages de forma especial
			anthropicReq.System = msg.Content
		} else {
			anthropicMsg := AnthropicMessage{
				Role: msg.Role,
			}

			// Manejar mensajes con tool calls
			if len(msg.ToolCalls) > 0 {
				// Para mensajes de assistant con tool calls
				content := make([]AnthropicContent, 0)
				
				// Agregar texto si existe
				if msg.Content != "" {
					content = append(content, AnthropicContent{
						Type: "text",
						Text: msg.Content,
					})
				}

				// Agregar tool uses
				for _, toolCall := range msg.ToolCalls {
					var input interface{}
					if err := json.Unmarshal([]byte(toolCall.Function.Arguments), &input); err != nil {
						// Si no se puede parsear, usar como string
						input = toolCall.Function.Arguments
					}

					content = append(content, AnthropicContent{
						Type:  "tool_use",
						ID:    toolCall.ID,
						Name:  toolCall.Function.Name,
						Input: input,
					})
				}
				
				anthropicMsg.Content = content
			} else if msg.ToolCallID != "" {
				// Para mensajes de tool results - Anthropic los maneja como mensajes de user
				anthropicMsg.Role = "user"
				anthropicMsg.Content = []AnthropicContent{
					{
						Type:      "tool_result",
						ToolUseID: msg.ToolCallID,
						Content:   msg.Content,
					},
				}
			} else {
				// Mensaje normal de texto
				anthropicMsg.Content = msg.Content
			}

			anthropicReq.Messages = append(anthropicReq.Messages, anthropicMsg)
		}
	}

	return anthropicReq
}

// handleHTTPError maneja errores HTTP específicos de Anthropic
func (p *AnthropicProvider) handleHTTPError(statusCode int, body []byte) error {
	var errorResp AnthropicError
	if err := json.Unmarshal(body, &errorResp); err != nil {
		return &ProviderError{
			Provider: p.GetName(),
			Type:     ErrorTypeServerError,
			Message:  fmt.Sprintf("HTTP %d: %s", statusCode, string(body)),
		}
	}

	errorType := ErrorTypeServerError
	switch statusCode {
	case 401:
		errorType = ErrorTypeAuth
	case 429:
		errorType = ErrorTypeRateLimit
	case 400:
		errorType = ErrorTypeInvalidReq
	}

	return &ProviderError{
		Provider: p.GetName(),
		Type:     errorType,
		Message:  errorResp.Error.Message,
	}
}

// Estructuras específicas de Anthropic

type AnthropicRequest struct {
	Model       string             `json:"model"`
	MaxTokens   int                `json:"max_tokens"`
	Temperature float64            `json:"temperature,omitempty"`
	Messages    []AnthropicMessage `json:"messages"`
	System      string             `json:"system,omitempty"`
	Tools       []AnthropicTool    `json:"tools,omitempty"`
}

type AnthropicMessage struct {
	Role      string                    `json:"role"`
	Content   interface{}               `json:"content"` // Can be string or []AnthropicContent
	ToolCalls []AnthropicToolCall       `json:"tool_calls,omitempty"`
}

type AnthropicContent struct {
	Type     string                `json:"type"`
	Text     string                `json:"text,omitempty"`
	// For tool_use type, the fields are directly in this object
	ID       string                `json:"id,omitempty"`
	Name     string                `json:"name,omitempty"`
	Input    interface{}           `json:"input,omitempty"`
	// For tool_result type
	ToolUseID string               `json:"tool_use_id,omitempty"`
	Content   string               `json:"content,omitempty"`
}

type AnthropicToolUse struct {
	ID    string      `json:"id"`
	Name  string      `json:"name"`
	Input interface{} `json:"input"`
}

type AnthropicToolResult struct {
	ToolUseID string `json:"tool_use_id"`
	Content   string `json:"content"`
}

type AnthropicTool struct {
	Name        string                    `json:"name"`
	Description string                    `json:"description"`
	InputSchema AnthropicToolInputSchema  `json:"input_schema"`
}

type AnthropicToolInputSchema struct {
	Type       string                 `json:"type"`
	Properties map[string]interface{} `json:"properties"`
	Required   []string               `json:"required,omitempty"`
}

type AnthropicToolCall struct {
	ID       string      `json:"id"`
	Type     string      `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

type AnthropicResponse struct {
	ID      string             `json:"id"`
	Type    string             `json:"type"`
	Role    string             `json:"role"`
	Content []AnthropicContent `json:"content"`
	Model   string             `json:"model"`
	Usage   struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
	StopReason string `json:"stop_reason"`
}

type AnthropicError struct {
	Type  string `json:"type"`
	Error struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error"`
}