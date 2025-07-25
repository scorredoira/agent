package llm

import (
	"context"
	"time"
)

// LoggedProvider envuelve un provider para capturar todas las interacciones
type LoggedProvider struct {
	provider  Provider
	logger    *InteractionLogger
	sessionID string
}

// NewLoggedProvider crea un provider con logging
func NewLoggedProvider(provider Provider, logger *InteractionLogger, sessionID string) *LoggedProvider {
	return &LoggedProvider{
		provider:  provider,
		logger:    logger,
		sessionID: sessionID,
	}
}

// GetName devuelve el nombre del provider subyacente
func (lp *LoggedProvider) GetName() string {
	return lp.provider.GetName()
}

// IsAvailable verifica disponibilidad sin logging (para evitar spam)
func (lp *LoggedProvider) IsAvailable(ctx context.Context) bool {
	return lp.provider.IsAvailable(ctx)
}

// Complete ejecuta y loggea la interacción completa
func (lp *LoggedProvider) Complete(ctx context.Context, req *CompletionRequest) (*CompletionResponse, error) {
	start := time.Now()
	
	// Ejecutar la request
	resp, err := lp.provider.Complete(ctx, req)
	duration := time.Since(start)
	
	// Loggear la interacción
	lp.logger.LogInteraction(ctx, lp.sessionID, lp.provider.GetName(), req, resp, err, duration)
	
	return resp, err
}

// Stream ejecuta y loggea el streaming (aproximado)
func (lp *LoggedProvider) Stream(ctx context.Context, req *CompletionRequest) (<-chan StreamChunk, error) {
	start := time.Now()
	
	// Ejecutar la request de streaming
	streamCh, err := lp.provider.Stream(ctx, req)
	if err != nil {
		duration := time.Since(start)
		lp.logger.LogInteraction(ctx, lp.sessionID, lp.provider.GetName(), req, nil, err, duration)
		return nil, err
	}
	
	// Crear canal que intercepte los chunks para logging
	loggedCh := make(chan StreamChunk, 10)
	
	go func() {
		defer close(loggedCh)
		
		var content string
		
		for chunk := range streamCh {
			content += chunk.Content
			loggedCh <- chunk
		}
		
		// Loggear cuando termine el streaming
		duration := time.Since(start)
		
		// Crear respuesta simulada para el log
		var resp *CompletionResponse
		if content != "" {
			resp = &CompletionResponse{
				Content:      content,
				Model:        req.Model,
				ResponseTime: duration,
				// Usage se deja vacío ya que en streaming no siempre está disponible
			}
		}
		
		lp.logger.LogInteraction(ctx, lp.sessionID, lp.provider.GetName(), req, resp, nil, duration)
	}()
	
	return loggedCh, nil
}

// GetModels delega al provider subyacente
func (lp *LoggedProvider) GetModels() []string {
	return lp.provider.GetModels()
}

// GetDefaultModel delega al provider subyacente
func (lp *LoggedProvider) GetDefaultModel() string {
	return lp.provider.GetDefaultModel()
}

// ValidateConfig delega al provider subyacente
func (lp *LoggedProvider) ValidateConfig() error {
	return lp.provider.ValidateConfig()
}

// SupportsFunctionCalling delega al provider subyacente
func (lp *LoggedProvider) SupportsFunctionCalling() bool {
	return lp.provider.SupportsFunctionCalling()
}

// SetSessionID cambia el ID de sesión para nuevos logs
func (lp *LoggedProvider) SetSessionID(sessionID string) {
	lp.sessionID = sessionID
}

// GetSessionID devuelve el ID de sesión actual
func (lp *LoggedProvider) GetSessionID() string {
	return lp.sessionID
}

// GetLogger devuelve el logger para acceso externo
func (lp *LoggedProvider) GetLogger() *InteractionLogger {
	return lp.logger
}