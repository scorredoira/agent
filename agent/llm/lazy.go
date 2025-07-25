package llm

import (
	"context"
	"fmt"
	"os"
	"sync"
	"time"
)

// LazyProvider implements lazy loading and background testing of LLM providers
type LazyProvider struct {
	mu              sync.RWMutex
	activeProvider  Provider
	providers       []Provider
	current         int
	isInitializing  bool
	backgroundDone  chan struct{}
}

// NewLazyProvider creates a new lazy provider that loads providers in background
func NewLazyProvider(providers []Provider) *LazyProvider {
	lp := &LazyProvider{
		providers:      providers,
		current:        -1,
		backgroundDone: make(chan struct{}),
	}
	
	// Start background initialization
	go lp.initializeProvidersInBackground()
	
	return lp
}

// GetName returns the name of the active provider, or "lazy" if still loading
func (lp *LazyProvider) GetName() string {
	lp.mu.RLock()
	defer lp.mu.RUnlock()
	
	if lp.activeProvider != nil {
		return lp.activeProvider.GetName()
	}
	return "lazy(loading...)"
}

// GetDefaultModel returns the default model of the active provider
func (lp *LazyProvider) GetDefaultModel() string {
	lp.mu.RLock()
	defer lp.mu.RUnlock()
	
	if lp.activeProvider != nil {
		return lp.activeProvider.GetDefaultModel()
	}
	return "unknown"
}

// GetModels returns the models of the active provider
func (lp *LazyProvider) GetModels() []string {
	lp.mu.RLock()
	defer lp.mu.RUnlock()
	
	if lp.activeProvider != nil {
		return lp.activeProvider.GetModels()
	}
	return []string{"loading..."}
}

// ValidateConfig validates the configuration of all providers
func (lp *LazyProvider) ValidateConfig() error {
	lp.mu.RLock()
	defer lp.mu.RUnlock()
	
	for _, provider := range lp.providers {
		if err := provider.ValidateConfig(); err != nil {
			return fmt.Errorf("provider %s: %w", provider.GetName(), err)
		}
	}
	return nil
}

// SupportsFunctionCalling indicates if any provider supports function calling
func (lp *LazyProvider) SupportsFunctionCalling() bool {
	lp.mu.RLock()
	defer lp.mu.RUnlock()
	
	for _, provider := range lp.providers {
		if provider.SupportsFunctionCalling() {
			return true
		}
	}
	return false
}

// Stream method for streaming responses
func (lp *LazyProvider) Stream(ctx context.Context, req *CompletionRequest) (<-chan StreamChunk, error) {
	// Ensure we have an active provider
	if !lp.ensureActiveProvider(ctx) {
		return nil, fmt.Errorf("no LLM providers available")
	}
	
	lp.mu.RLock()
	provider := lp.activeProvider
	lp.mu.RUnlock()
	
	return provider.Stream(ctx, req)
}

// IsAvailable checks if any provider is available
func (lp *LazyProvider) IsAvailable(ctx context.Context) bool {
	lp.mu.RLock()
	if lp.activeProvider != nil {
		defer lp.mu.RUnlock()
		return lp.activeProvider.IsAvailable(ctx)
	}
	lp.mu.RUnlock()
	
	// If no active provider, try to get one
	return lp.ensureActiveProvider(ctx)
}

// Complete executes a completion request
func (lp *LazyProvider) Complete(ctx context.Context, req *CompletionRequest) (*CompletionResponse, error) {
	// Ensure we have an active provider
	if !lp.ensureActiveProvider(ctx) {
		return nil, fmt.Errorf("no LLM providers available")
	}
	
	lp.mu.RLock()
	provider := lp.activeProvider
	lp.mu.RUnlock()
	
	return provider.Complete(ctx, req)
}

// ensureActiveProvider makes sure we have an active provider available
func (lp *LazyProvider) ensureActiveProvider(ctx context.Context) bool {
	lp.mu.RLock()
	if lp.activeProvider != nil {
		defer lp.mu.RUnlock()
		return true
	}
	lp.mu.RUnlock()
	
	// Try to find an available provider
	lp.mu.Lock()
	defer lp.mu.Unlock()
	
	// Double-check after acquiring write lock
	if lp.activeProvider != nil {
		return true
	}
	
	// Try providers in order
	for i, provider := range lp.providers {
		// Test availability with even longer timeout for web context
		testCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
		if os.Getenv("LLM_DEBUG") != "" {
			fmt.Fprintf(os.Stderr, "Testing provider %s...\n", provider.GetName())
		}
		if provider.IsAvailable(testCtx) {
			lp.activeProvider = provider
			lp.current = i
			cancel()
			if os.Getenv("LLM_DEBUG") != "" {
				fmt.Fprintf(os.Stderr, "✅ Using provider %s (provider #%d)\n", provider.GetName(), i+1)
			}
			return true
		}
		cancel()
		// Log failed attempt with more detail
		if os.Getenv("LLM_DEBUG") != "" {
			fmt.Fprintf(os.Stderr, "❌ Provider %s unavailable (timeout after 20s), trying next...\n", provider.GetName())
		}
	}
	
	return false
}

// initializeProvidersInBackground tests all providers in parallel in the background
func (lp *LazyProvider) initializeProvidersInBackground() {
	defer close(lp.backgroundDone)
	
	lp.mu.Lock()
	lp.isInitializing = true
	lp.mu.Unlock()
	
	// Create context with longer timeout for background testing
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	
	// Test all providers in parallel
	var wg sync.WaitGroup
	results := make([]bool, len(lp.providers))
	
	for i, provider := range lp.providers {
		wg.Add(1)
		go func(idx int, p Provider) {
			defer wg.Done()
			
			// Test with reasonable timeout
			testCtx, testCancel := context.WithTimeout(ctx, 5*time.Second)
			available := p.IsAvailable(testCtx)
			testCancel()
			
			results[idx] = available
		}(i, provider)
	}
	
	wg.Wait()
	
	// Set first available provider as active
	lp.mu.Lock()
	if lp.activeProvider == nil {
		for i, available := range results {
			if available {
				lp.activeProvider = lp.providers[i]
				lp.current = i
				if os.Getenv("LLM_DEBUG") != "" {
					fmt.Fprintf(os.Stderr, "LLM Provider initialized: %s\n", lp.providers[i].GetName())
				}
				break
			} else if os.Getenv("LLM_DEBUG") != "" {
				fmt.Fprintf(os.Stderr, "Provider %s not available during initialization\n", lp.providers[i].GetName())
			}
		}
	}
	lp.isInitializing = false
	lp.mu.Unlock()
}

// GetProviderStatus returns the status of all providers (for debugging)
func (lp *LazyProvider) GetProviderStatus() map[string]bool {
	lp.mu.RLock()
	defer lp.mu.RUnlock()
	
	status := make(map[string]bool)
	for i, provider := range lp.providers {
		name := provider.GetName()
		status[name] = (i == lp.current && lp.activeProvider != nil)
	}
	return status
}

// WaitForBackgroundInit waits for background initialization to complete
func (lp *LazyProvider) WaitForBackgroundInit(timeout time.Duration) bool {
	select {
	case <-lp.backgroundDone:
		return true
	case <-time.After(timeout):
		return false
	}
}