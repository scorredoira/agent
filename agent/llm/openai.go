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

// OpenAIProvider implementa el proveedor para OpenAI
type OpenAIProvider struct {
	config     *Config
	httpClient *http.Client
}

// NewOpenAIProvider crea una nueva instancia del proveedor OpenAI
func NewOpenAIProvider(config *Config) *OpenAIProvider {
	if config.BaseURL == "" {
		config.BaseURL = "https://api.openai.com"
	}
	if config.Model == "" {
		config.Model = "gpt-4o"
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

	return &OpenAIProvider{
		config: config,
		httpClient: &http.Client{
			Timeout: config.Timeout,
		},
	}
}

// GetName devuelve el nombre del proveedor
func (p *OpenAIProvider) GetName() string {
	return "openai"
}

// IsAvailable verifica si el proveedor está disponible
func (p *OpenAIProvider) IsAvailable(ctx context.Context) bool {
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
func (p *OpenAIProvider) Complete(ctx context.Context, req *CompletionRequest) (*CompletionResponse, error) {
	start := time.Now()

	// Preparar la solicitud para la API de OpenAI
	openaiReq := p.buildOpenAIRequest(req)
	
	jsonData, err := json.Marshal(openaiReq)
	if err != nil {
		return nil, &ProviderError{
			Provider: p.GetName(),
			Type:     ErrorTypeInvalidReq,
			Message:  "failed to marshal request",
			Err:      err,
		}
	}

	// Crear la solicitud HTTP
	httpReq, err := http.NewRequestWithContext(ctx, "POST", p.config.BaseURL+"/v1/chat/completions", bytes.NewBuffer(jsonData))
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
	httpReq.Header.Set("Authorization", "Bearer "+p.config.APIKey)

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

	// Parsear la respuesta
	var openaiResp OpenAIResponse
	if err := json.Unmarshal(body, &openaiResp); err != nil {
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
	
	if len(openaiResp.Choices) > 0 {
		choice := openaiResp.Choices[0]
		content = choice.Message.Content
		
		// Convertir tool calls si están presentes
		if len(choice.Message.ToolCalls) > 0 {
			toolCalls = make([]ToolCall, 0, len(choice.Message.ToolCalls))
			for _, tc := range choice.Message.ToolCalls {
				toolCalls = append(toolCalls, ToolCall{
					ID:   tc.ID,
					Type: tc.Type,
					Function: FunctionCall{
						Name:      tc.Function.Name,
						Arguments: tc.Function.Arguments,
					},
				})
			}
		}
	}

	return &CompletionResponse{
		Content:      content,
		Model:        openaiResp.Model,
		ToolCalls:    toolCalls,
		Usage: TokenUsage{
			PromptTokens:     openaiResp.Usage.PromptTokens,
			CompletionTokens: openaiResp.Usage.CompletionTokens,
			TotalTokens:      openaiResp.Usage.TotalTokens,
		},
		ResponseTime: time.Since(start),
	}, nil
}

// Stream implementa streaming para OpenAI
func (p *OpenAIProvider) Stream(ctx context.Context, req *CompletionRequest) (<-chan StreamChunk, error) {
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
func (p *OpenAIProvider) GetModels() []string {
	return []string{
		"gpt-4o",
		"gpt-4o-mini",
		"gpt-4-turbo",
		"gpt-4",
		"gpt-3.5-turbo",
	}
}

// GetDefaultModel devuelve el modelo por defecto
func (p *OpenAIProvider) GetDefaultModel() string {
	return "gpt-4o"
}

// ValidateConfig valida la configuración
func (p *OpenAIProvider) ValidateConfig() error {
	if p.config.APIKey == "" {
		return fmt.Errorf("API key is required for OpenAI provider")
	}
	return nil
}

// SupportsFunctionCalling indica si OpenAI soporta function calling
func (p *OpenAIProvider) SupportsFunctionCalling() bool {
	return true
}

// buildOpenAIRequest convierte nuestra solicitud al formato de OpenAI
func (p *OpenAIProvider) buildOpenAIRequest(req *CompletionRequest) *OpenAIRequest {
	openaiReq := &OpenAIRequest{
		Model:       req.Model,
		MaxTokens:   req.MaxTokens,
		Temperature: req.Temperature,
		Messages:    make([]OpenAIMessage, 0, len(req.Messages)),
	}

	// Si no se especifica modelo, usar el por defecto
	if openaiReq.Model == "" {
		openaiReq.Model = p.config.Model
	}
	if openaiReq.MaxTokens == 0 {
		openaiReq.MaxTokens = p.config.MaxTokens
	}
	if openaiReq.Temperature == 0 {
		openaiReq.Temperature = p.config.Temperature
	}

	// Convertir mensajes al formato de OpenAI
	for _, msg := range req.Messages {
		openaiMsg := OpenAIMessage{
			Role:    msg.Role,
			Content: msg.Content,
		}
		
		// Convertir ToolCalls si están presentes
		if len(msg.ToolCalls) > 0 {
			openaiMsg.ToolCalls = make([]OpenAIToolCall, 0, len(msg.ToolCalls))
			for _, tc := range msg.ToolCalls {
				openaiMsg.ToolCalls = append(openaiMsg.ToolCalls, OpenAIToolCall{
					ID:   tc.ID,
					Type: tc.Type,
					Function: OpenAIFunctionCall{
						Name:      tc.Function.Name,
						Arguments: tc.Function.Arguments,
					},
				})
			}
		}
		
		// Agregar ToolCallID si está presente
		if msg.ToolCallID != "" {
			openaiMsg.ToolCallID = msg.ToolCallID
		}
		
		openaiReq.Messages = append(openaiReq.Messages, openaiMsg)
	}

	// Convertir tools si están presentes
	if len(req.Tools) > 0 {
		openaiReq.Tools = make([]OpenAITool, 0, len(req.Tools))
		for _, tool := range req.Tools {
			openaiReq.Tools = append(openaiReq.Tools, OpenAITool{
				Type: tool.Type,
				Function: OpenAIFunctionDefinition{
					Name:        tool.Function.Name,
					Description: tool.Function.Description,
					Parameters:  tool.Function.Parameters,
				},
			})
		}
		
		// Set tool choice if specified
		if req.ToolChoice != "" {
			openaiReq.ToolChoice = req.ToolChoice
		} else {
			openaiReq.ToolChoice = "auto"
		}
	}

	return openaiReq
}

// handleHTTPError maneja errores HTTP específicos de OpenAI
func (p *OpenAIProvider) handleHTTPError(statusCode int, body []byte) error {
	var errorResp OpenAIError
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

// Estructuras específicas de OpenAI

type OpenAIRequest struct {
	Model       string          `json:"model"`
	Messages    []OpenAIMessage `json:"messages"`
	MaxTokens   int             `json:"max_tokens,omitempty"`
	Temperature float64         `json:"temperature,omitempty"`
	Tools       []OpenAITool    `json:"tools,omitempty"`
	ToolChoice  interface{}     `json:"tool_choice,omitempty"`
}

type OpenAIMessage struct {
	Role      string              `json:"role"`
	Content   string              `json:"content,omitempty"`
	ToolCalls []OpenAIToolCall    `json:"tool_calls,omitempty"`
	ToolCallID string             `json:"tool_call_id,omitempty"`
}

type OpenAITool struct {
	Type     string                    `json:"type"`
	Function OpenAIFunctionDefinition `json:"function"`
}

type OpenAIFunctionDefinition struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Parameters  map[string]interface{} `json:"parameters"`
}

type OpenAIToolCall struct {
	ID       string                `json:"id"`
	Type     string                `json:"type"`
	Function OpenAIFunctionCall   `json:"function"`
}

type OpenAIFunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

type OpenAIResponse struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	Model   string `json:"model"`
	Choices []struct {
		Index   int `json:"index"`
		Message struct {
			Role      string            `json:"role"`
			Content   string            `json:"content"`
			ToolCalls []OpenAIToolCall  `json:"tool_calls,omitempty"`
		} `json:"message"`
		FinishReason string `json:"finish_reason"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
}

type OpenAIError struct {
	Error struct {
		Message string `json:"message"`
		Type    string `json:"type"`
		Code    string `json:"code"`
	} `json:"error"`
}