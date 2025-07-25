package cache

import (
	"crypto/sha256"
	"fmt"
	"sync"
	"time"

	"github.com/santiagocorredoira/agent/agent/tools"
)

// PromptCacheEntry represents a cached system prompt
type PromptCacheEntry struct {
	Prompt    string
	Timestamp time.Time
	Hash      string
}

// PromptCache manages cached system prompts to avoid regeneration
type PromptCache struct {
	cache  map[string]*PromptCacheEntry
	mutex  sync.RWMutex
	maxAge time.Duration
}

// NewPromptCache creates a new prompt cache
func NewPromptCache(maxAge time.Duration) *PromptCache {
	if maxAge == 0 {
		maxAge = 30 * time.Minute // Default 30 minutes
	}
	
	return &PromptCache{
		cache:  make(map[string]*PromptCacheEntry),
		maxAge: maxAge,
	}
}

// GeneratePromptKey creates a cache key based on prompt configuration
func (pc *PromptCache) GeneratePromptKey(hasTools bool, toolsOnlyMode bool, toolCount int) string {
	return fmt.Sprintf("prompt_%t_%t_%d", hasTools, toolsOnlyMode, toolCount)
}

// GenerateContextKey creates a cache key for user context
func (pc *PromptCache) GenerateContextKey(userName, organization, apiHost string) string {
	h := sha256.New()
	h.Write([]byte(fmt.Sprintf("%s:%s:%s", userName, organization, apiHost)))
	return fmt.Sprintf("ctx_%x", h.Sum(nil)[:8])
}

// Get retrieves a cached prompt if available and valid
func (pc *PromptCache) Get(key string) (string, bool) {
	pc.mutex.RLock()
	defer pc.mutex.RUnlock()
	
	entry, exists := pc.cache[key]
	if !exists {
		return "", false
	}
	
	// Check if cache entry is still valid
	if time.Since(entry.Timestamp) > pc.maxAge {
		// Cache expired, will be cleaned up later
		return "", false
	}
	
	return entry.Prompt, true
}

// Set stores a prompt in the cache
func (pc *PromptCache) Set(key, prompt string) {
	pc.mutex.Lock()
	defer pc.mutex.Unlock()
	
	pc.cache[key] = &PromptCacheEntry{
		Prompt:    prompt,
		Timestamp: time.Now(),
		Hash:      pc.generateHash(prompt),
	}
}

// InvalidateByPattern removes cache entries matching a pattern
func (pc *PromptCache) InvalidateByPattern(pattern string) {
	pc.mutex.Lock()
	defer pc.mutex.Unlock()
	
	keysToDelete := make([]string, 0)
	for key := range pc.cache {
		// Simple pattern matching - for more complex patterns use regex
		if len(pattern) > 0 && len(key) >= len(pattern) && key[:len(pattern)] == pattern {
			keysToDelete = append(keysToDelete, key)
		}
	}
	
	for _, key := range keysToDelete {
		delete(pc.cache, key)
	}
}

// InvalidateOnToolChange invalidates prompt cache when tools change
func (pc *PromptCache) InvalidateOnToolChange() {
	pc.InvalidateByPattern("prompt_")
}

// CleanExpired removes expired cache entries
func (pc *PromptCache) CleanExpired() {
	pc.mutex.Lock()
	defer pc.mutex.Unlock()
	
	now := time.Now()
	keysToDelete := make([]string, 0)
	
	for key, entry := range pc.cache {
		if now.Sub(entry.Timestamp) > pc.maxAge {
			keysToDelete = append(keysToDelete, key)
		}
	}
	
	for _, key := range keysToDelete {
		delete(pc.cache, key)
	}
}

// GetStats returns cache statistics
func (pc *PromptCache) GetStats() map[string]interface{} {
	pc.mutex.RLock()
	defer pc.mutex.RUnlock()
	
	return map[string]interface{}{
		"total_entries": len(pc.cache),
		"max_age_minutes": int(pc.maxAge.Minutes()),
	}
}

// generateHash creates a hash of the prompt content
func (pc *PromptCache) generateHash(content string) string {
	h := sha256.New()
	h.Write([]byte(content))
	return fmt.Sprintf("%x", h.Sum(nil)[:8])
}

// StartCleanupRoutine starts a background goroutine to clean expired entries
func (pc *PromptCache) StartCleanupRoutine() {
	go func() {
		ticker := time.NewTicker(5 * time.Minute) // Clean every 5 minutes
		defer ticker.Stop()
		
		for range ticker.C {
			pc.CleanExpired()
		}
	}()
}

// ToolStateHash generates a hash representing the current tool state
func (pc *PromptCache) ToolStateHash(toolRegistry *tools.ToolRegistry) string {
	if toolRegistry == nil {
		return "no_tools"
	}
	
	// This would need to be implemented in the tools package
	// For now, use a simple tool count approach
	return fmt.Sprintf("tools_%d", len(toolRegistry.ListAvailableTools(nil)))
}