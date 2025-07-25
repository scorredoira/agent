package llm

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// MockScenario represents a testing scenario for the mock provider
type MockScenario struct {
	Name        string
	Steps       []MockStep
	Description string
}

// MockStep represents a single step in a mock scenario  
type MockStep struct {
	Input            string
	MockResponse     CompletionResponse
	MockError        string
	ExpectedDuration time.Duration
}

// MockProvider implementa un proveedor simulado para testing
type MockProvider struct {
	config        *Config
	responses     []string
	responseIndex int
	shouldFail    bool
	errorType     string
	latency       time.Duration
	scenario      *MockScenario
	stepIndex     int
}

// NewMockProvider crea una nueva instancia del proveedor mock
func NewMockProvider(config *Config) *MockProvider {
	if config == nil {
		config = &Config{
			Model:       "mock-model",
			MaxTokens:   4096,
			Temperature: 0.7,
		}
	}

	return &MockProvider{
		config: config,
		responses: []string{
			"This is a mock response from the AI agent.",
			"I understand you're testing the system. Everything looks good!",
			"Mock provider is working correctly. Ready for more complex tasks.",
			"I can help you configure your club settings and diagnose issues.",
			"Based on the documentation, here's what I found...",
		},
		responseIndex: 0,
		shouldFail:    false,
		latency:       100 * time.Millisecond, // Simular latencia real
		stepIndex:     0,
	}
}

// GetName devuelve el nombre del proveedor
func (p *MockProvider) GetName() string {
	return "mock"
}

// IsAvailable siempre devuelve true para el mock
func (p *MockProvider) IsAvailable(ctx context.Context) bool {
	if p.shouldFail {
		return false
	}
	return true
}

// Complete simula una respuesta de completado
func (p *MockProvider) Complete(ctx context.Context, req *CompletionRequest) (*CompletionResponse, error) {
	// Si hay un escenario activo, usar la respuesta del escenario
	if p.scenario != nil && p.stepIndex < len(p.scenario.Steps) {
		return p.completeFromScenario(ctx, req)
	}

	// Simular latencia
	select {
	case <-time.After(p.latency):
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	// Simular error si está configurado
	if p.shouldFail {
		return nil, &ProviderError{
			Provider: p.GetName(),
			Type:     p.errorType,
			Message:  "simulated error for testing",
		}
	}

	// Generar respuesta basada en el input
	content := p.generateResponse(req)
	
	// Calcular tokens estimados
	promptTokens := len(strings.Fields(strings.Join(getMessageContents(req.Messages), " ")))
	completionTokens := len(strings.Fields(content))
	
	return &CompletionResponse{
		Content: content,
		Model:   p.config.Model,
		Usage: TokenUsage{
			PromptTokens:     promptTokens,
			CompletionTokens: completionTokens,
			TotalTokens:      promptTokens + completionTokens,
		},
		ResponseTime: p.latency,
	}, nil
}

// Stream simula streaming de respuesta
func (p *MockProvider) Stream(ctx context.Context, req *CompletionRequest) (<-chan StreamChunk, error) {
	ch := make(chan StreamChunk, 10)
	
	if p.shouldFail {
		go func() {
			defer close(ch)
			ch <- StreamChunk{
				Content: fmt.Sprintf("Error: %s", p.errorType), 
				Done:    true,
			}
		}()
		return ch, nil
	}
	
	go func() {
		defer close(ch)
		
		content := p.generateResponse(req)
		words := strings.Fields(content)
		
		for _, word := range words {
			select {
			case <-ctx.Done():
				return
			case ch <- StreamChunk{Content: word + " ", Done: false}:
				time.Sleep(50 * time.Millisecond) // Simular streaming real
			}
		}
		
		ch <- StreamChunk{Content: "", Done: true}
	}()
	
	return ch, nil
}

// GetModels devuelve modelos simulados
func (p *MockProvider) GetModels() []string {
	return []string{
		"mock-model",
		"mock-fast",
		"mock-smart",
		"mock-creative",
	}
}

// GetDefaultModel devuelve el modelo por defecto
func (p *MockProvider) GetDefaultModel() string {
	return "mock-model"
}

// ValidateConfig siempre valida correctamente
func (p *MockProvider) ValidateConfig() error {
	return nil
}

// SupportsFunctionCalling indica si Mock soporta function calling
func (p *MockProvider) SupportsFunctionCalling() bool {
	return true // Mock puede simular function calling
}

// generateResponse genera una respuesta basada en el contexto
func (p *MockProvider) generateResponse(req *CompletionRequest) string {
	// Si no hay mensajes, usar respuesta por defecto
	if len(req.Messages) == 0 {
		return p.getNextResponse()
	}
	
	// Si hay respuestas personalizadas y no estamos usando lógica contextual,
	// usar las respuestas personalizadas directamente
	if len(p.responses) > 0 {
		lastMessage := req.Messages[len(req.Messages)-1].Content
		lowered := strings.ToLower(lastMessage)
		
		// Solo usar lógica contextual para mensajes específicos, de lo contrario usar respuestas personalizadas
		switch {
		case strings.Contains(lowered, "hello") || strings.Contains(lowered, "hi"):
			return "Hello! I'm a mock AI agent ready to help you with your club management tasks."
			
		case strings.Contains(lowered, "price") || strings.Contains(lowered, "pricing"):
			return "I can help you configure pricing for your club. The pricing system allows you to set different rates for members, guests, and special events. Would you like me to show you the current pricing configuration?"
			
		case strings.Contains(lowered, "club") || strings.Contains(lowered, "configure"):
			return "I can help you configure various aspects of your club including: membership tiers, pricing, facilities, booking rules, and user permissions. What specific area would you like to configure?"
			
		case strings.Contains(lowered, "error") || strings.Contains(lowered, "problem"):
			return "I'll help you diagnose the issue. Let me check the system logs and configuration. Common issues include: incorrect pricing rules, permission conflicts, or cached data. Can you provide more details about what users are experiencing?"
			
		case strings.Contains(lowered, "api") || strings.Contains(lowered, "endpoint"):
			return "Based on the API documentation, here are the relevant endpoints: /api/clubs/{id}/pricing, /api/members/{id}, /api/bookings. Each endpoint supports GET, POST, PUT, and DELETE operations with proper authentication."
			
		default:
			// Para cualquier otro mensaje, usar las respuestas personalizadas en secuencia
			return p.getNextResponse()
		}
	}
	
	// Si no hay respuestas personalizadas, usar respuesta por defecto
	return "Mock response generated successfully."
}

// getNextResponse devuelve la siguiente respuesta en secuencia
func (p *MockProvider) getNextResponse() string {
	if len(p.responses) == 0 {
		return "Mock response generated successfully."
	}
	
	response := p.responses[p.responseIndex]
	p.responseIndex = (p.responseIndex + 1) % len(p.responses)
	return response
}

// getMessageContents extrae el contenido de todos los mensajes
func getMessageContents(messages []Message) []string {
	contents := make([]string, len(messages))
	for i, msg := range messages {
		contents[i] = msg.Content
	}
	return contents
}

// Métodos de configuración para testing

// SetResponses configura respuestas personalizadas
func (p *MockProvider) SetResponses(responses []string) {
	p.responses = responses
	p.responseIndex = 0
}

// SetShouldFail configura el mock para simular errores
func (p *MockProvider) SetShouldFail(shouldFail bool, errorType string) {
	p.shouldFail = shouldFail
	p.errorType = errorType
}

// SetLatency configura la latencia simulada
func (p *MockProvider) SetLatency(latency time.Duration) {
	p.latency = latency
}

// Reset reinicia el estado del mock
func (p *MockProvider) Reset() {
	p.responseIndex = 0
	p.shouldFail = false
	p.errorType = ""
	p.latency = 100 * time.Millisecond
	p.scenario = nil
	p.stepIndex = 0
}

// SetScenario configura un escenario para reproducir
func (p *MockProvider) SetScenario(scenario *MockScenario) {
	p.scenario = scenario
	p.stepIndex = 0
}

// GetScenario devuelve el escenario actual
func (p *MockProvider) GetScenario() *MockScenario {
	return p.scenario
}

// GetCurrentStep devuelve el paso actual del escenario
func (p *MockProvider) GetCurrentStep() int {
	return p.stepIndex
}

// IsScenarioComplete indica si el escenario ha terminado
func (p *MockProvider) IsScenarioComplete() bool {
	return p.scenario != nil && p.stepIndex >= len(p.scenario.Steps)
}

// completeFromScenario maneja respuestas basadas en el escenario activo
func (p *MockProvider) completeFromScenario(ctx context.Context, req *CompletionRequest) (*CompletionResponse, error) {
	if p.scenario == nil || p.stepIndex >= len(p.scenario.Steps) {
		return nil, &ProviderError{
			Provider: p.GetName(),
			Type:     ErrorTypeServerError,
			Message:  "scenario complete or not set",
		}
	}

	step := p.scenario.Steps[p.stepIndex]
	p.stepIndex++

	// Simular la latencia esperada del paso
	latency := step.ExpectedDuration
	if latency <= 0 {
		latency = p.latency
	}

	select {
	case <-time.After(latency):
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	// Si el paso tiene un error definido, devolverlo
	if step.MockError != "" {
		return nil, &ProviderError{
			Provider: p.GetName(),
			Type:     ErrorTypeServerError,
			Message:  step.MockError,
		}
	}

	// Devolver la respuesta del escenario
	response := step.MockResponse
	response.ResponseTime = latency
	return &response, nil
}