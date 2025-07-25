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

// GeminiProvider implementa el proveedor para Google Gemini
type GeminiProvider struct {
	config     *Config
	httpClient *http.Client
}

// NewGeminiProvider crea una nueva instancia del proveedor Gemini
func NewGeminiProvider(config *Config) *GeminiProvider {
	if config.BaseURL == "" {
		config.BaseURL = "https://generativelanguage.googleapis.com"
	}
	if config.Model == "" {
		config.Model = "gemini-1.5-pro"
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

	return &GeminiProvider{
		config: config,
		httpClient: &http.Client{
			Timeout: config.Timeout,
		},
	}
}

// GetName devuelve el nombre del proveedor
func (p *GeminiProvider) GetName() string {
	return "gemini"
}

// IsAvailable verifica si el proveedor está disponible
func (p *GeminiProvider) IsAvailable(ctx context.Context) bool {
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
func (p *GeminiProvider) Complete(ctx context.Context, req *CompletionRequest) (*CompletionResponse, error) {
	start := time.Now()

	// Preparar la solicitud para la API de Gemini
	geminiReq := p.buildGeminiRequest(req)
	
	jsonData, err := json.Marshal(geminiReq)
	if err != nil {
		return nil, &ProviderError{
			Provider: p.GetName(),
			Type:     ErrorTypeInvalidReq,
			Message:  "failed to marshal request",
			Err:      err,
		}
	}

	// Construir URL con API key
	url := fmt.Sprintf("%s/v1beta/models/%s:generateContent?key=%s", 
		p.config.BaseURL, p.config.Model, p.config.APIKey)

	// Crear la solicitud HTTP
	httpReq, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(jsonData))
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
	var geminiResp GeminiResponse
	if err := json.Unmarshal(body, &geminiResp); err != nil {
		return nil, &ProviderError{
			Provider: p.GetName(),
			Type:     ErrorTypeServerError,
			Message:  "failed to parse response",
			Err:      err,
		}
	}

	// Convertir a nuestro formato estándar
	content := ""
	if len(geminiResp.Candidates) > 0 && len(geminiResp.Candidates[0].Content.Parts) > 0 {
		content = geminiResp.Candidates[0].Content.Parts[0].Text
	}

	// Gemini no siempre devuelve usage info, usar valores estimados
	totalTokens := len(strings.Fields(content)) * 2 // Estimación simple
	promptTokens := totalTokens / 3                 // Estimación simple

	return &CompletionResponse{
		Content: content,
		Model:   p.config.Model,
		Usage: TokenUsage{
			PromptTokens:     promptTokens,
			CompletionTokens: totalTokens - promptTokens,
			TotalTokens:      totalTokens,
		},
		ResponseTime: time.Since(start),
	}, nil
}

// Stream implementa streaming para Gemini
func (p *GeminiProvider) Stream(ctx context.Context, req *CompletionRequest) (<-chan StreamChunk, error) {
	ch := make(chan StreamChunk, 10)
	
	// Por ahora, implementamos streaming simulado usando Complete
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
func (p *GeminiProvider) GetModels() []string {
	return []string{
		"gemini-1.5-pro",
		"gemini-1.5-flash",
		"gemini-1.0-pro",
	}
}

// GetDefaultModel devuelve el modelo por defecto
func (p *GeminiProvider) GetDefaultModel() string {
	return "gemini-1.5-pro"
}

// ValidateConfig valida la configuración
func (p *GeminiProvider) ValidateConfig() error {
	if p.config.APIKey == "" {
		return fmt.Errorf("API key is required for Gemini provider")
	}
	return nil
}

// SupportsFunctionCalling indica si Gemini soporta function calling
func (p *GeminiProvider) SupportsFunctionCalling() bool {
	return false // TODO: Implementar function calling para Gemini
}

// buildGeminiRequest convierte nuestra solicitud al formato de Gemini
func (p *GeminiProvider) buildGeminiRequest(req *CompletionRequest) *GeminiRequest {
	geminiReq := &GeminiRequest{
		Contents: make([]GeminiContent, 0),
		GenerationConfig: &GeminiGenerationConfig{
			Temperature:     req.Temperature,
			MaxOutputTokens: req.MaxTokens,
		},
	}

	// Si no se especifican valores, usar los por defecto
	if geminiReq.GenerationConfig.Temperature == 0 {
		geminiReq.GenerationConfig.Temperature = p.config.Temperature
	}
	if geminiReq.GenerationConfig.MaxOutputTokens == 0 {
		geminiReq.GenerationConfig.MaxOutputTokens = p.config.MaxTokens
	}

	// Convertir mensajes al formato de Gemini
	for _, msg := range req.Messages {
		role := msg.Role
		if role == "assistant" {
			role = "model"
		}
		
		geminiReq.Contents = append(geminiReq.Contents, GeminiContent{
			Role: role,
			Parts: []GeminiPart{
				{Text: msg.Content},
			},
		})
	}

	return geminiReq
}

// handleHTTPError maneja errores HTTP específicos de Gemini
func (p *GeminiProvider) handleHTTPError(statusCode int, body []byte) error {
	var errorResp GeminiError
	if err := json.Unmarshal(body, &errorResp); err != nil {
		return &ProviderError{
			Provider: p.GetName(),
			Type:     ErrorTypeServerError,
			Message:  fmt.Sprintf("HTTP %d: %s", statusCode, string(body)),
		}
	}

	errorType := ErrorTypeServerError
	switch statusCode {
	case 401, 403:
		errorType = ErrorTypeAuth
	case 429:
		errorType = ErrorTypeRateLimit
	case 400:
		errorType = ErrorTypeInvalidReq
	}

	message := "unknown error"
	if errorResp.Error.Message != "" {
		message = errorResp.Error.Message
	}

	return &ProviderError{
		Provider: p.GetName(),
		Type:     errorType,
		Message:  message,
	}
}

// Estructuras específicas de Gemini

type GeminiRequest struct {
	Contents         []GeminiContent          `json:"contents"`
	GenerationConfig *GeminiGenerationConfig  `json:"generationConfig,omitempty"`
}

type GeminiContent struct {
	Role  string      `json:"role"`
	Parts []GeminiPart `json:"parts"`
}

type GeminiPart struct {
	Text string `json:"text"`
}

type GeminiGenerationConfig struct {
	Temperature     float64 `json:"temperature,omitempty"`
	MaxOutputTokens int     `json:"maxOutputTokens,omitempty"`
}

type GeminiResponse struct {
	Candidates []struct {
		Content struct {
			Parts []struct {
				Text string `json:"text"`
			} `json:"parts"`
			Role string `json:"role"`
		} `json:"content"`
		FinishReason string `json:"finishReason"`
	} `json:"candidates"`
}

type GeminiError struct {
	Error struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Status  string `json:"status"`
	} `json:"error"`
}