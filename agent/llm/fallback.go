package llm

import (
	"context"
	"fmt"
	"time"
)

// FallbackProvider implementa un sistema de fallback entre múltiples proveedores
type FallbackProvider struct {
	providers []Provider
	timeout   time.Duration
}

// NewFallbackProvider crea un nuevo proveedor con sistema de fallback
func NewFallbackProvider(providers []Provider, timeout time.Duration) *FallbackProvider {
	if timeout == 0 {
		timeout = 30 * time.Second
	}
	
	return &FallbackProvider{
		providers: providers,
		timeout:   timeout,
	}
}

// GetName devuelve el nombre del proveedor compuesto
func (f *FallbackProvider) GetName() string {
	if len(f.providers) == 0 {
		return "fallback-empty"
	}
	
	names := make([]string, len(f.providers))
	for i, p := range f.providers {
		names[i] = p.GetName()
	}
	
	return fmt.Sprintf("fallback[%v]", names)
}

// IsAvailable verifica si al menos un proveedor está disponible
func (f *FallbackProvider) IsAvailable(ctx context.Context) bool {
	ctx, cancel := context.WithTimeout(ctx, f.timeout)
	defer cancel()
	
	for _, provider := range f.providers {
		if provider.IsAvailable(ctx) {
			return true
		}
	}
	return false
}

// Complete intenta completar usando proveedores en orden hasta que uno funcione
func (f *FallbackProvider) Complete(ctx context.Context, req *CompletionRequest) (*CompletionResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, f.timeout)
	defer cancel()
	
	var lastError error
	
	for i, provider := range f.providers {
		// Verificar si el proveedor está disponible antes de intentar
		if !provider.IsAvailable(ctx) {
			lastError = &ProviderError{
				Provider: provider.GetName(),
				Type:     ErrorTypeNetwork,
				Message:  "provider not available",
			}
			continue
		}
		
		// Intentar completar con este proveedor
		resp, err := provider.Complete(ctx, req)
		if err == nil {
			// Éxito - agregar información de qué proveedor se usó
			resp.Model = fmt.Sprintf("%s (%s)", resp.Model, provider.GetName())
			return resp, nil
		}
		
		// Si es un error de autenticación o quota, intentar el siguiente proveedor
		if providerErr, ok := err.(*ProviderError); ok {
			switch providerErr.Type {
			case ErrorTypeAuth, ErrorTypeQuotaExceed, ErrorTypeRateLimit:
				lastError = err
				continue
			}
		}
		
		// Para otros errores, si no es el último proveedor, intentar el siguiente
		if i < len(f.providers)-1 {
			lastError = err
			continue
		}
		
		// Si es el último proveedor, devolver el error
		return nil, err
	}
	
	// Ningún proveedor funcionó
	if lastError != nil {
		return nil, &ProviderError{
			Provider: f.GetName(),
			Type:     ErrorTypeServerError,
			Message:  "all providers failed",
			Err:      lastError,
		}
	}
	
	return nil, &ProviderError{
		Provider: f.GetName(),
		Type:     ErrorTypeServerError,
		Message:  "no providers configured",
	}
}

// Stream intenta hacer streaming usando el primer proveedor disponible
func (f *FallbackProvider) Stream(ctx context.Context, req *CompletionRequest) (<-chan StreamChunk, error) {
	ctx, cancel := context.WithTimeout(ctx, f.timeout)
	defer cancel()
	
	for _, provider := range f.providers {
		if provider.IsAvailable(ctx) {
			return provider.Stream(ctx, req)
		}
	}
	
	// Si ningún proveedor está disponible, devolver un canal con error
	ch := make(chan StreamChunk, 1)
	go func() {
		defer close(ch)
		ch <- StreamChunk{
			Content: "Error: no providers available for streaming",
			Done:    true,
		}
	}()
	
	return ch, &ProviderError{
		Provider: f.GetName(),
		Type:     ErrorTypeNetwork,
		Message:  "no providers available for streaming",
	}
}

// GetModels devuelve todos los modelos disponibles de todos los proveedores
func (f *FallbackProvider) GetModels() []string {
	var allModels []string
	seen := make(map[string]bool)
	
	for _, provider := range f.providers {
		models := provider.GetModels()
		for _, model := range models {
			// Agregar prefijo del proveedor para evitar duplicados
			qualifiedModel := fmt.Sprintf("%s/%s", provider.GetName(), model)
			if !seen[qualifiedModel] {
				allModels = append(allModels, qualifiedModel)
				seen[qualifiedModel] = true
			}
		}
	}
	
	return allModels
}

// GetDefaultModel devuelve el modelo por defecto del primer proveedor
func (f *FallbackProvider) GetDefaultModel() string {
	if len(f.providers) == 0 {
		return ""
	}
	
	firstProvider := f.providers[0]
	return fmt.Sprintf("%s/%s", firstProvider.GetName(), firstProvider.GetDefaultModel())
}

// ValidateConfig valida la configuración de todos los proveedores
func (f *FallbackProvider) ValidateConfig() error {
	if len(f.providers) == 0 {
		return fmt.Errorf("no providers configured for fallback")
	}
	
	var errors []string
	for _, provider := range f.providers {
		if err := provider.ValidateConfig(); err != nil {
			errors = append(errors, fmt.Sprintf("%s: %v", provider.GetName(), err))
		}
	}
	
	if len(errors) == len(f.providers) {
		// Todos los proveedores tienen errores de configuración
		return fmt.Errorf("all providers have configuration errors: %v", errors)
	}
	
	// Al menos un proveedor está bien configurado
	return nil
}

// SupportsFunctionCalling indica si algún proveedor soporta function calling
func (f *FallbackProvider) SupportsFunctionCalling() bool {
	for _, provider := range f.providers {
		if provider.SupportsFunctionCalling() {
			return true
		}
	}
	return false
}

// GetAvailableProviders devuelve una lista de proveedores disponibles
func (f *FallbackProvider) GetAvailableProviders(ctx context.Context) []Provider {
	var available []Provider
	
	for _, provider := range f.providers {
		if provider.IsAvailable(ctx) {
			available = append(available, provider)
		}
	}
	
	return available
}

// AddProvider agrega un nuevo proveedor al final de la lista de fallback
func (f *FallbackProvider) AddProvider(provider Provider) {
	f.providers = append(f.providers, provider)
}

// RemoveProvider remueve un proveedor por nombre
func (f *FallbackProvider) RemoveProvider(name string) bool {
	for i, provider := range f.providers {
		if provider.GetName() == name {
			f.providers = append(f.providers[:i], f.providers[i+1:]...)
			return true
		}
	}
	return false
}

// ReorderProviders cambia el orden de los proveedores
func (f *FallbackProvider) ReorderProviders(names []string) error {
	if len(names) != len(f.providers) {
		return fmt.Errorf("names count (%d) doesn't match providers count (%d)", len(names), len(f.providers))
	}
	
	newProviders := make([]Provider, 0, len(names))
	providerMap := make(map[string]Provider)
	
	// Crear mapa de nombre -> proveedor
	for _, provider := range f.providers {
		providerMap[provider.GetName()] = provider
	}
	
	// Reordenar según los nombres proporcionados
	for _, name := range names {
		provider, exists := providerMap[name]
		if !exists {
			return fmt.Errorf("provider %s not found", name)
		}
		newProviders = append(newProviders, provider)
	}
	
	f.providers = newProviders
	return nil
}

// GetProviderStats devuelve estadísticas de uso de los proveedores
func (f *FallbackProvider) GetProviderStats(ctx context.Context) map[string]bool {
	stats := make(map[string]bool)
	
	for _, provider := range f.providers {
		stats[provider.GetName()] = provider.IsAvailable(ctx)
	}
	
	return stats
}