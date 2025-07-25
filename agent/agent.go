package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/santiagocorredoira/agent/agent/cache"
	"github.com/santiagocorredoira/agent/agent/config"
	"github.com/santiagocorredoira/agent/agent/llm"
	"github.com/santiagocorredoira/agent/agent/memory"
	"github.com/santiagocorredoira/agent/agent/planner"
	"github.com/santiagocorredoira/agent/agent/prompts"
	"github.com/santiagocorredoira/agent/agent/tools"
)

// V3Agent represents the core agent instance for library usage
type V3Agent struct {
	config         *config.Config
	llmProvider    llm.Provider
	memoryManager  *memory.MemoryManager
	toolRegistry   *tools.ToolRegistry
	taskPlanner    *planner.TaskPlanner
	ctx            context.Context
	cancel         context.CancelFunc
	currentSession *memory.ConversationMemory
	toolsOnlyMode  bool                   // If true, only respond to questions requiring tools
	contextInfo    *ConversationContext   // Context for personalization, added to first message
	contextAdded   bool                   // Track if context has been added to avoid duplicates
	logger         *llm.InteractionLogger // Optional interaction logger
	promptCache    *cache.PromptCache     // Cache for system prompts
}

// AgentConfig provides configuration options for the agent
type AgentConfig struct {
	ConfigPath       string
	StorageDir       string
	AutoSave         bool
	MaxSessions      int
	CustomTools      []tools.Tool
	ContextProviders []memory.ContextProvider
	ToolsOnlyMode    bool // If true, only respond to questions requiring tools (default: true)
}

// ConversationContext provides contextual information about the user/session
type ConversationContext struct {
	UserName     string            `json:"user_name,omitempty"`    // User's name for personalization
	Organization string            `json:"organization,omitempty"` // User's company/club/organization
	Role         string            `json:"role,omitempty"`         // User's role (admin, developer, etc.)
	Preferences  map[string]string `json:"preferences,omitempty"`  // Custom preferences (timezone, language, etc.)
	Metadata     map[string]any    `json:"metadata,omitempty"`     // Additional contextual data
}

// StatusCallback is called with status messages during processing
type StatusCallback func(message string)

// ConversationOptions provides options for conversations
type ConversationOptions struct {
	MaxTokens      int
	Temperature    float64
	SystemPrompt   string
	ContextLimit   int
	Context        *ConversationContext // Optional context for personalization
	StatusCallback StatusCallback       // Optional callback for status messages
}

// DefaultConversationOptions returns sensible defaults
func DefaultConversationOptions() ConversationOptions {
	return ConversationOptions{
		MaxTokens:    2500, // Increased for code examples and detailed explanations
		Temperature:  0.7,
		SystemPrompt: prompts.SystemBriefTemplate,
		ContextLimit: 4000,
	}
}

// NewV3Agent creates a new V3 Agent instance for library usage
func NewV3Agent(cfg AgentConfig) (*V3Agent, error) {
	// Load configuration
	configPath := cfg.ConfigPath
	if configPath == "" {
		configPath = "config.json"
	}
	agentConfig := config.LoadConfigOrDefault(configPath)

	// Create LLM provider
	provider, err := createLLMProvider(agentConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create LLM provider: %w", err)
	}

	// Create memory manager
	storageDir := cfg.StorageDir
	if storageDir == "" {
		storageDir = "./agent_memory"
	}
	memoryManager := memory.NewMemoryManager(storageDir)
	if err := memoryManager.Initialize(); err != nil {
		return nil, fmt.Errorf("failed to initialize memory: %w", err)
	}

	// Create tool registry and register basic tools
	toolRegistry := tools.NewToolRegistry()
	if err := registerBasicTools(toolRegistry, agentConfig, provider); err != nil {
		return nil, fmt.Errorf("failed to register basic tools: %w", err)
	}

	// Register custom tools if provided
	for _, tool := range cfg.CustomTools {
		if err := toolRegistry.RegisterTool(tool); err != nil {
			return nil, fmt.Errorf("failed to register custom tool %s: %w", tool.GetName(), err)
		}
	}

	// Register custom context providers if provided
	for _, provider := range cfg.ContextProviders {
		memoryManager.RegisterContextProvider(provider)
	}

	// Create task planner
	taskPlanner := planner.NewTaskPlanner(toolRegistry)

	// Create context with cancellation
	ctx, cancel := context.WithCancel(context.Background())

	// Create prompt cache
	promptCache := cache.NewPromptCache(30 * time.Minute) // 30 minute cache
	promptCache.StartCleanupRoutine()

	agent := &V3Agent{
		config:        agentConfig,
		llmProvider:   provider,
		memoryManager: memoryManager,
		toolRegistry:  toolRegistry,
		taskPlanner:   taskPlanner,
		ctx:           ctx,
		cancel:        cancel,
		toolsOnlyMode: cfg.ToolsOnlyMode, // Use the configuration value
		promptCache:   promptCache,
	}

	return agent, nil
}

// StartConversation starts a new conversation session
func (a *V3Agent) StartConversation() (*memory.ConversationMemory, error) {
	return a.StartConversationWithContext(nil)
}

// StartConversationWithContext starts a new conversation session with optional context
func (a *V3Agent) StartConversationWithContext(context *ConversationContext) (*memory.ConversationMemory, error) {
	session, err := a.memoryManager.StartNewSession()
	if err != nil {
		return nil, fmt.Errorf("failed to start conversation: %w", err)
	}

	// Set this as current session
	a.currentSession = session

	// Store context for later use (will be added with first message)
	a.contextInfo = context
	a.contextAdded = false

	return session, nil
}

// LoadConversation loads an existing conversation by session ID
func (a *V3Agent) LoadConversation(sessionID string) (*memory.ConversationMemory, error) {
	session, err := a.memoryManager.LoadSession(sessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to load conversation %s: %w", sessionID, err)
	}
	a.currentSession = session
	return session, nil
}

// DeleteConversation deletes a conversation by session ID
func (a *V3Agent) DeleteConversation(sessionID string) error {
	// Clear current session if it's the one being deleted
	if a.currentSession != nil && a.currentSession.SessionID == sessionID {
		a.currentSession = nil
	}

	// Remove session from memory manager
	err := a.memoryManager.DeleteSession(sessionID)
	if err != nil {
		return fmt.Errorf("failed to delete conversation %s: %w", sessionID, err)
	}

	return nil
}

// SetLogger sets the interaction logger for the agent
func (a *V3Agent) SetLogger(logger *llm.InteractionLogger) {
	a.logger = logger
}

// GetLogger returns the interaction logger for the agent
func (a *V3Agent) GetLogger() *llm.InteractionLogger {
	return a.logger
}

// GetConfig returns the agent configuration
func (a *V3Agent) GetConfig() *config.Config {
	return a.config
}

// SendMessage sends a message to the agent and returns the response
func (a *V3Agent) SendMessage(message string, options ...ConversationOptions) (*llm.CompletionResponse, error) {
	return a.sendMessageInternal(message, false, options...)
}

// SendMessageWithStreaming sends a message with real-time progress feedback
func (a *V3Agent) SendMessageWithStreaming(message string, options ...ConversationOptions) (*llm.CompletionResponse, error) {
	return a.sendMessageInternal(message, true, options...)
}

// sendMessageInternal is the internal implementation that handles both streaming and non-streaming
func (a *V3Agent) sendMessageInternal(message string, enableStreaming bool, options ...ConversationOptions) (*llm.CompletionResponse, error) {
	if a.currentSession == nil {
		return nil, fmt.Errorf("no active conversation session")
	}

	// Use default options if none provided
	opts := DefaultConversationOptions()
	if len(options) > 0 {
		opts = options[0]
	}

	// DEBUG: Log provider state at start of message processing
	log.Printf("🔍 sendMessageInternal START - Provider: %s", a.llmProvider.GetName())
	log.Printf("🔍 Provider available check: %t", a.llmProvider.IsAvailable(a.ctx))

	// Show initial status if streaming enabled
	if enableStreaming {
		if opts.StatusCallback != nil {
			opts.StatusCallback("Processing request...")
		} else {
			fmt.Printf("Processing request...\n")
		}
	}

	// Context is now included directly in the system prompt via cache
	// No need to add as separate message
	a.contextAdded = true

	// Add user message to memory
	userMessage := llm.Message{Role: "user", Content: message}
	if err := a.memoryManager.AddMessageToCurrentSession(userMessage); err != nil {
		return nil, fmt.Errorf("failed to add message to session: %w", err)
	}

	// Build tools list for LLM function calling
	if enableStreaming {
		if opts.StatusCallback != nil {
			opts.StatusCallback("Preparing tools...")
		} else {
			fmt.Printf("Preparing tools...\n")
		}
	}
	availableTools := a.buildToolsForLLM()

	var toolContext string

	// Get contextual messages
	if enableStreaming {
		if opts.StatusCallback != nil {
			opts.StatusCallback("Building context...")
		} else {
			fmt.Printf("Building context...\n")
		}
	}
	contextMessages := a.memoryManager.GetContextForQuery(message, opts.ContextLimit)

	// Add tool context if available
	if toolContext != "" {
		toolMessage := llm.Message{Role: "system", Content: toolContext}
		contextMessages = append(contextMessages, toolMessage)
	}

	// Add system prompt (combine default agent prompt with user's custom prompt)
	systemPrompt := a.buildSystemPromptCached(len(availableTools) > 0)
	if opts.SystemPrompt != "" {
		systemPrompt += "\n\nAdditional instructions: " + opts.SystemPrompt
	}

	systemMessage := llm.Message{Role: "system", Content: systemPrompt}
	contextMessages = append([]llm.Message{systemMessage}, contextMessages...)

	// Create completion request
	req := &llm.CompletionRequest{
		Messages:    contextMessages,
		MaxTokens:   opts.MaxTokens,
		Temperature: opts.Temperature,
		Tools:       availableTools,
		ToolChoice:  "auto", // Let LLM decide when to use tools
	}

	// Get response from LLM with simple robust error handling
	if enableStreaming {
		if opts.StatusCallback != nil {
			opts.StatusCallback("Thinking...")
		} else {
			fmt.Printf("Thinking...\n")
		}
	}

	// DEBUG: Log before calling Complete
	log.Printf("🔍 About to call Complete - Provider: %s", a.llmProvider.GetName())

	resp, err := a.llmProvider.Complete(a.ctx, req)

	// DEBUG: Log after calling Complete
	log.Printf("🔍 Complete returned - Error: %v", err)
	if resp != nil {
		log.Printf("🔍 Response content preview: %.100s", resp.Content)
	}

	if err != nil {
		// Simple fallback response instead of failing
		log.Printf("🔍 Using fallback response due to error: %v", err)
		resp = &llm.CompletionResponse{
			Content: fmt.Sprintf("I'm experiencing technical difficulties. Error: %v\n\nPlease try rephrasing your question or ask something simpler.", err),
			Usage: llm.TokenUsage{
				PromptTokens:     0,
				CompletionTokens: 0,
				TotalTokens:      0,
			},
		}
	} else {
		// Handle tool calls if LLM requested them
		if len(resp.ToolCalls) > 0 {
			if enableStreaming {
				message := fmt.Sprintf("Searching documentation (%d searches)...", len(resp.ToolCalls))
				if opts.StatusCallback != nil {
					opts.StatusCallback(message)
				} else {
					fmt.Printf("%s\n", message)
				}
			}
			toolResp, toolErr := a.handleToolCallsWithDepthLimitStreaming(resp, contextMessages, opts, 0, enableStreaming)
			if toolErr != nil {
				// If tool calls fail, provide a fallback response with the original content
				resp.Content = fmt.Sprintf("%s\n\n⚠️ I attempted to use tools but encountered an issue: %v\nI can help with general questions without external data.", resp.Content, toolErr)
			} else {
				resp = toolResp
			}
		}
	}

	// Add assistant response to memory
	assistantMessage := llm.Message{Role: "assistant", Content: resp.Content}
	if err := a.memoryManager.AddMessageToCurrentSession(assistantMessage); err != nil {
		return nil, fmt.Errorf("failed to add response to session: %w", err)
	}

	// Generate AI summary asynchronously if conversation has enough messages
	if a.currentSession != nil && len(a.currentSession.Messages) >= 6 {
		// Check if we need to generate/update the summary
		needsSummary := a.currentSession.Summary == "" ||
			strings.Contains(a.currentSession.Summary, "(comprimida)") ||
			strings.Contains(a.currentSession.Summary, "conversación general")

		if needsSummary {
			go func() {
				if err := a.GenerateConversationSummary(a.currentSession.SessionID); err != nil {
					log.Printf("Failed to generate conversation summary: %v", err)
				}
			}()
		}
	}

	return resp, nil
}

// ExecuteTool executes a tool with the given parameters
func (a *V3Agent) ExecuteTool(toolName string, parameters map[string]interface{}, confirmed bool) (*tools.ToolResult, error) {
	execution := tools.ToolExecution{
		ToolName:   toolName,
		Parameters: parameters,
		Confirmed:  confirmed,
		Timestamp:  time.Now(),
	}

	if a.currentSession != nil {
		execution.SessionID = a.currentSession.SessionID
	}

	result, err := a.toolRegistry.ExecuteTool(a.ctx, execution)
	if err != nil {
		return nil, fmt.Errorf("tool execution failed: %w", err)
	}

	return result, nil
}

// GetAvailableTools returns a list of available tools
func (a *V3Agent) GetAvailableTools() []tools.ToolInfo {
	return a.toolRegistry.ListAvailableTools(a.ctx)
}

// GetToolsByCategory returns tools filtered by category
func (a *V3Agent) GetToolsByCategory(category tools.ToolCategory) []tools.ToolInfo {
	return a.toolRegistry.GetToolsByCategory(a.ctx, category)
}

// SearchTools searches for tools by name or description
func (a *V3Agent) SearchTools(query string) []tools.ToolInfo {
	return a.toolRegistry.SearchTools(a.ctx, query)
}

// GetCurrentSession returns the current conversation session
func (a *V3Agent) GetCurrentSession() *memory.ConversationMemory {
	return a.currentSession
}

// ListConversations returns a list of all conversation sessions
func (a *V3Agent) ListConversations() ([]memory.SessionSummary, error) {
	return a.memoryManager.ListSessions()
}

// SearchConversations searches through conversation history
func (a *V3Agent) SearchConversations(query string, limit int) ([]memory.SearchResult, error) {
	return a.memoryManager.SearchConversations(query, limit)
}

// GetStats returns agent statistics
func (a *V3Agent) GetStats() AgentStats {
	memStats := a.memoryManager.GetGlobalStats()
	toolStats := a.toolRegistry.GetToolStats()

	// Safe conversion with defaults
	totalSessions := 0
	if val, ok := memStats["total_sessions"]; ok && val != nil {
		if sessions, ok := val.(int); ok {
			totalSessions = sessions
		}
	}

	currentMessages := 0
	if val, ok := memStats["current_messages"]; ok && val != nil {
		if messages, ok := val.(int); ok {
			currentMessages = messages
		}
	}

	contextProviders := 0
	if val, ok := memStats["context_providers"]; ok && val != nil {
		if providers, ok := val.([]memory.ProviderInfo); ok {
			contextProviders = len(providers)
		}
	}

	return AgentStats{
		TotalSessions:    totalSessions,
		CurrentMessages:  currentMessages,
		ContextProviders: contextProviders,
		TotalTools:       toolStats.TotalTools,
		AvailableTools:   toolStats.AvailableTools,
		ToolExecutions:   toolStats.TotalExecutions,
		ToolSuccessRate:  toolStats.OverallSuccessRate,
		LLMProvider:      a.llmProvider.GetName(),
		LLMAvailable:     a.llmProvider.IsAvailable(a.ctx),
	}
}

// AgentStats provides statistics about the agent
type AgentStats struct {
	TotalSessions    int     `json:"total_sessions"`
	CurrentMessages  int     `json:"current_messages"`
	ContextProviders int     `json:"context_providers"`
	TotalTools       int     `json:"total_tools"`
	AvailableTools   int     `json:"available_tools"`
	ToolExecutions   int     `json:"tool_executions"`
	ToolSuccessRate  float64 `json:"tool_success_rate"`
	LLMProvider      string  `json:"llm_provider"`
	LLMAvailable     bool    `json:"llm_available"`
}

// AddBusinessContext adds a business-specific context provider
func (a *V3Agent) AddBusinessContext(domain string, keywords []string, context string) {
	a.memoryManager.AddBusinessContext(domain, keywords, context)
}

// AddConfigurableContext adds a fully configurable context provider
func (a *V3Agent) AddConfigurableContext(name, description string, config map[string]interface{}) {
	a.memoryManager.AddConfigurableContext(name, description, config)
}

// RegisterTool registers a new tool
func (a *V3Agent) RegisterTool(tool tools.Tool) error {
	err := a.toolRegistry.RegisterTool(tool)
	if err == nil {
		// Invalidate prompt cache when tools change
		a.promptCache.InvalidateOnToolChange()
	}
	return err
}

// EnableInteractionLogging configures the LLM provider to log all interactions
func (a *V3Agent) EnableInteractionLogging(logger *llm.InteractionLogger, sessionID string) {
	if logger == nil {
		return
	}

	// Check if already wrapped with LoggedProvider to avoid double-wrapping
	if loggedProvider, ok := a.llmProvider.(*llm.LoggedProvider); ok {
		// Already wrapped, just update the session ID
		loggedProvider.SetSessionID(sessionID)
		return
	}

	// Wrap the current provider with logging
	a.llmProvider = llm.NewLoggedProvider(a.llmProvider, logger, sessionID)
}

// UnregisterTool removes a tool from the registry
func (a *V3Agent) UnregisterTool(name string) bool {
	success := a.toolRegistry.UnregisterTool(name)
	if success {
		// Invalidate prompt cache when tools change
		a.promptCache.InvalidateOnToolChange()
	}
	return success
}

// GenerateConversationSummary generates an AI-powered summary for a conversation
func (a *V3Agent) GenerateConversationSummary(sessionID string) error {
	// Try to use current session first if it matches
	var session *memory.ConversationMemory
	var err error

	if a.currentSession != nil && a.currentSession.SessionID == sessionID {
		session = a.currentSession
	} else {
		session, err = a.memoryManager.LoadSession(sessionID)
		if err != nil {
			return fmt.Errorf("failed to load session for summary: %w", err)
		}
	}

	// Generate AI summary
	if err := session.GenerateAISummary(a.ctx, a.llmProvider); err != nil {
		return fmt.Errorf("failed to generate AI summary: %w", err)
	}

	// Save the session with updated summary
	if err := session.Save(); err != nil {
		return fmt.Errorf("failed to save session with summary: %w", err)
	}

	return nil
}

// GenerateConversationTitle generates a short descriptive title for a conversation
func (a *V3Agent) GenerateConversationTitle(sessionID string) (string, error) {
	// Try to use current session first if it matches
	var session *memory.ConversationMemory
	var err error

	if a.currentSession != nil && a.currentSession.SessionID == sessionID {
		session = a.currentSession
	} else {
		session, err = a.memoryManager.LoadSession(sessionID)
		if err != nil {
			return "New conversation", nil // Fail silently with default title
		}
	}

	// Get the first few messages to understand the context
	messages := session.GetRecentMessages(6) // First 3 user-assistant exchanges typically
	if len(messages) == 0 {
		return "New conversation", nil
	}

	// Find the first user message to base the title on
	var firstUserMessage string
	for _, msg := range messages {
		if msg.Role == "user" {
			firstUserMessage = msg.Content
			break
		}
	}

	if firstUserMessage == "" {
		return "New conversation", nil
	}

	// Create a prompt to generate a short title using template
	titlePrompt := prompts.RenderConversationTitlePrompt(prompts.PromptData{
		FirstUserMessage: firstUserMessage,
	})

	// Create a simple request for title generation
	req := &llm.CompletionRequest{
		Messages: []llm.Message{
			{Role: "user", Content: titlePrompt},
		},
		MaxTokens:   20,  // Very short response
		Temperature: 0.3, // Lower temperature for more consistent titles
	}

	resp, err := a.llmProvider.Complete(a.ctx, req)
	if err != nil {
		// Fallback to a simple title based on first words
		words := strings.Fields(firstUserMessage)
		if len(words) > 4 {
			words = words[:4]
		}
		return strings.Join(words, " ") + "...", nil
	}

	title := strings.TrimSpace(resp.Content)

	// Clean up the title
	title = strings.Trim(title, `"'`)
	if len(title) > 50 {
		title = title[:47] + "..."
	}

	if title == "" {
		title = "New conversation"
	}

	return title, nil
}

// WaitForReady waits until the agent is fully initialized and ready to use
func (a *V3Agent) WaitForReady(maxWaitTime time.Duration) error {
	start := time.Now()
	for {
		if a.llmProvider.IsAvailable(a.ctx) {
			log.Printf("Agent is ready after %v", time.Since(start))
			return nil
		}

		if time.Since(start) > maxWaitTime {
			return fmt.Errorf("agent not ready after %v timeout", maxWaitTime)
		}

		time.Sleep(100 * time.Millisecond)
	}
}

// Shutdown gracefully shuts down the agent
func (a *V3Agent) Shutdown() error {
	// Cancel context to stop any ongoing operations
	a.cancel()

	// Save current session if exists
	if a.currentSession != nil {
		if err := a.currentSession.Save(); err != nil {
			return fmt.Errorf("failed to save current session: %w", err)
		}
	}

	return nil
}

// Helper functions

func createLLMProvider(cfg *config.Config) (llm.Provider, error) {
	var providers []llm.Provider

	// Create providers in fallback order without testing availability
	for _, providerName := range cfg.LLM.FallbackOrder {
		if !cfg.LLM.Providers[providerName].Enabled {
			continue
		}

		llmConfig := cfg.GetLLMConfig(providerName)
		if llmConfig == nil {
			continue
		}

		var provider llm.Provider
		switch providerName {
		case "anthropic":
			provider = llm.NewAnthropicProvider(llmConfig)
		case "openai":
			provider = llm.NewOpenAIProvider(llmConfig)
		case "gemini":
			provider = llm.NewGeminiProvider(llmConfig)
		case "mock":
			provider = llm.NewMockProvider(llmConfig)
		default:
			continue
		}

		providers = append(providers, provider)
	}

	if len(providers) == 0 {
		return nil, fmt.Errorf("no LLM providers configured")
	}

	// Return lazy provider that will test availability in background
	return llm.NewLazyProvider(providers), nil
}

func registerBasicTools(registry *tools.ToolRegistry, config *config.Config, llmProvider llm.Provider) error {
	// Only register knowledge base tools if path is configured and exists
	if config.KnowledgeBase.Path != "" {
		// Check if path exists
		if _, err := os.Stat(config.KnowledgeBase.Path); err == nil {
			// Create restricted filesystem for knowledge base
			restrictedFS, err := tools.NewRestrictedFS(config.KnowledgeBase.Path)
			if err != nil {
				return fmt.Errorf("failed to create restricted filesystem: %w", err)
			}

			// Register the new search engine tool with LLM provider
			searchEngineTool := tools.NewSearchEngineToolWithLLM(restrictedFS, llmProvider)
			if err := registry.RegisterTool(searchEngineTool); err != nil {
				return fmt.Errorf("failed to register search engine tool: %w", err)
			}
		} else {
			log.Printf("Knowledge base path %s does not exist, skipping knowledge base tools", config.KnowledgeBase.Path)
		}
	}

	return nil
}

// buildToolsForLLM builds the list of available tools for LLM function calling
func (a *V3Agent) buildToolsForLLM() []llm.FunctionTool {
	availableTools := a.toolRegistry.ListAvailableTools(a.ctx)
	tools := make([]llm.FunctionTool, 0, len(availableTools))

	for _, toolInfo := range availableTools {
		// Get the actual tool to access GetFunctionDefinition
		tool, exists := a.toolRegistry.GetTool(toolInfo.Name)
		if !exists {
			continue
		}

		functionDef := tool.GetFunctionDefinition()
		tools = append(tools, llm.FunctionTool{
			Type:     "function",
			Function: functionDef,
		})
	}

	return tools
}

// handleToolCalls executes tools requested by the LLM and gets a final response
func (a *V3Agent) handleToolCalls(initialResp *llm.CompletionResponse, messages []llm.Message, opts ConversationOptions) (*llm.CompletionResponse, error) {
	// Add the assistant message with tool calls (ensure content is never null)
	content := initialResp.Content
	if content == "" {
		content = "I'll use some tools to help answer your question."
	}
	messages = append(messages, llm.Message{
		Role:      "assistant",
		Content:   content,
		ToolCalls: initialResp.ToolCalls,
	})

	// Execute each tool call
	for _, toolCall := range initialResp.ToolCalls {
		result, err := a.executeToolCall(toolCall)

		var toolContent string
		if err != nil {
			toolContent = fmt.Sprintf("Error executing %s: %v", toolCall.Function.Name, err)
		} else {
			toolContent = result
			if toolContent == "" {
				toolContent = "Tool executed successfully with no output."
			}
		}

		// Add tool result with proper tool_call_id
		messages = append(messages, llm.Message{
			Role:       "tool",
			Content:    toolContent,
			ToolCallID: toolCall.ID,
		})
	}

	// Get final response from LLM with tool results - Allow tools but limit depth
	finalReq := &llm.CompletionRequest{
		Messages:    messages,
		MaxTokens:   opts.MaxTokens, // Use full configured token limit for detailed responses
		Temperature: opts.Temperature,
		Tools:       a.buildToolsForLLM(), // Allow tools for complete responses
		ToolChoice:  "auto",
	}

	// Hard timeout - if this doesn't work, we return tool results directly
	ctx, cancel := context.WithTimeout(a.ctx, 30*time.Second)
	defer cancel()

	finalResp, err := a.llmProvider.Complete(ctx, finalReq)
	if err != nil {
		// Extract tool results and return them directly
		var toolResults []string
		for i := len(messages) - 1; i >= 0; i-- {
			if messages[i].Role == "tool" && messages[i].Content != "" {
				toolResults = append(toolResults, messages[i].Content)
			}
		}

		fallbackContent := "I found this information using the available tools:"
		if len(toolResults) > 0 {
			fallbackContent += "\n\n" + strings.Join(toolResults, "\n\n")
		} else {
			fallbackContent = "I attempted to use tools to help answer your question, but encountered technical difficulties. Please try rephrasing your question or ask for specific parts of the information you need."
		}

		return &llm.CompletionResponse{
			Content: fallbackContent,
			Usage: llm.TokenUsage{
				PromptTokens:     0,
				CompletionTokens: 0,
				TotalTokens:      0,
			},
		}, nil
	}

	// Handle nested tool calls if the response includes them, but with strict depth limit
	if len(finalResp.ToolCalls) > 0 {
		// Recursive tool calls with max depth 1 (only one more level)
		nestedResp, nestedErr := a.handleToolCallsWithDepthLimit(finalResp, messages, opts, 1)
		if nestedErr != nil {
			// If nested tools fail, return the current response with tool results
			return finalResp, nil
		}
		return nestedResp, nil
	}

	return finalResp, nil
}

// handleToolCallsWithDepthLimit handles tool calls with depth limiting to prevent infinite loops
func (a *V3Agent) handleToolCallsWithDepthLimit(initialResp *llm.CompletionResponse, messages []llm.Message, opts ConversationOptions, depth int) (*llm.CompletionResponse, error) {
	return a.handleToolCallsWithDepthLimitStreaming(initialResp, messages, opts, depth, false)
}

// handleToolCallsWithDepthLimitStreaming handles tool calls with optional progress streaming
func (a *V3Agent) handleToolCallsWithDepthLimitStreaming(initialResp *llm.CompletionResponse, messages []llm.Message, opts ConversationOptions, depth int, enableStreaming bool) (*llm.CompletionResponse, error) {
	maxDepth := 20   // Allow up to 20 tool calls to find information thoroughly
	minSearches := 8 // Minimum searches required - especially for synonym searches like bonos→vouchers

	// Count search attempts from messages
	searchCount := a.countSearchAttempts(messages)

	if depth >= maxDepth {
		// Force a complete response based on gathered information
		trimmedContent := strings.TrimSpace(initialResp.Content)
		if initialResp.Content == "" || strings.HasSuffix(trimmedContent, ":") || len(trimmedContent) < 100 {
			initialResp.Content = fmt.Sprintf("After exhaustive search (%d attempts), I could not find the specific information you requested in the available documentation. This may indicate:\n\n1. The information might be in a different location or format\n2. The documentation might be incomplete for this specific query\n3. The feature might not be documented yet\n\nPlease:\n- Try rephrasing your question with different keywords\n- Specify the exact API operation you need\n- Check if there are additional documentation sources\n\nSearch attempts made: %d", searchCount, searchCount)
		}
		return initialResp, nil
	}

	// Note: The forcing of additional searches is now handled later in the finalReq logic

	// Add the assistant message with tool calls
	content := initialResp.Content
	if content == "" {
		content = "I'll use some tools to help answer your question."
	}
	messages = append(messages, llm.Message{
		Role:      "assistant",
		Content:   content,
		ToolCalls: initialResp.ToolCalls,
	})

	// Execute each tool call
	for i, toolCall := range initialResp.ToolCalls {
		if enableStreaming {
			message := fmt.Sprintf("Searching %d/%d...", i+1, len(initialResp.ToolCalls))
			if opts.StatusCallback != nil {
				opts.StatusCallback(message)
			} else {
				fmt.Printf("%s\n", message)
			}
		}
		result, err := a.executeToolCall(toolCall)

		var toolContent string
		if err != nil {
			toolContent = fmt.Sprintf("Error executing %s: %v", toolCall.Function.Name, err)
		} else {
			toolContent = result
			if toolContent == "" {
				toolContent = "Tool executed successfully with no output."
			}
		}

		// Add tool result
		messages = append(messages, llm.Message{
			Role:       "tool",
			Content:    toolContent,
			ToolCallID: toolCall.ID,
		})
	}

	// Get final response from LLM with tool results
	if enableStreaming {
		if opts.StatusCallback != nil {
			opts.StatusCallback("Processing results...")
		} else {
			fmt.Printf("Processing results...\n")
		}
	}
	finalReq := &llm.CompletionRequest{
		Messages:    messages,
		MaxTokens:   opts.MaxTokens,
		Temperature: opts.Temperature,
	}

	// Re-check search count for forcing logic
	searchCount = a.countSearchAttempts(messages)

	// If we're at max depth, disable tools and add instruction to provide final answer
	if depth >= maxDepth-1 {
		finalReq.Tools = nil
		finalReq.ToolChoice = ""
		// Add instruction to provide final answer based on information already gathered
		if len(messages) > 0 {
			lastMessage := &messages[len(messages)-1]
			if lastMessage.Role == "assistant" {
				lastMessage.Content += "\n\nIMPORTANT: You've reached the maximum search depth. Provide a complete final answer based on the information you've already gathered. Do not promise further searches."
			}
		}
		if enableStreaming {
			if opts.StatusCallback != nil {
				opts.StatusCallback("Max search depth reached...")
			} else {
				fmt.Printf("Max search depth reached...\n")
			}
		}
	} else if searchCount < minSearches {
		// Check if recent searches found useful information (be stricter about what constitutes "useful")
		recentToolResults := 0
		recentToolContent := ""
		for i := len(messages) - 1; i >= 0 && i >= len(messages)-4; i-- { // Look at last 4 messages instead of 2
			if messages[i].Role == "tool" && len(messages[i].Content) > 200 { // Lower threshold for content length
				content := strings.ToLower(messages[i].Content)
				// Consider it useful if it contains ANY endpoint or API information
				if strings.Contains(content, "endpoint") || strings.Contains(content, "/api/") ||
					strings.Contains(content, "get ") || strings.Contains(content, "post ") ||
					strings.Contains(content, "put ") || strings.Contains(content, "delete ") ||
					strings.Contains(content, "http") || strings.Contains(content, "model/") {
					recentToolResults++
					recentToolContent += messages[i].Content + " "
				}
			}
		}

		// Only force more searches if we haven't found substantial technical results recently
		if recentToolResults == 0 || len(recentToolContent) < 500 {
			// Force tool usage if we haven't made enough searches AND haven't found useful info
			finalReq.Tools = a.buildToolsForLLM()
			finalReq.ToolChoice = "auto" // Keep auto but add strong prompt instruction
			// Add instruction to search more
			if len(messages) > 0 {
				lastMessage := &messages[len(messages)-1]
				if lastMessage.Role == "tool" {
					// Add a system message to force more searching with intelligent strategies
					forceSearchMsg := llm.Message{
						Role:    "system",
						Content: fmt.Sprintf("🚨 SEARCH REQUIREMENT 🚨\nYou have made %d/%d searches. Continue searching until you find useful information or reach %d total searches.\n\nINTELLIGENT SEARCH STRATEGIES:\n- Try business domain alternatives (bonus→voucher, reservation→booking)\n- Use singular/plural variations\n- Combine with API terms (endpoint, list, get, create)\n- Think conceptually: what business function does the user want?\n- Try abbreviated forms and technical variations\n\nIf you found useful information in recent searches, you may provide an answer. Otherwise, use the kbase tool with COMPLETELY DIFFERENT search terms.", searchCount, minSearches, minSearches),
					}
					messages = append(messages, forceSearchMsg)
					finalReq.Messages = messages
				}
			}
			if enableStreaming {
				message := fmt.Sprintf("Forcing more searches (%d/%d)...", searchCount, minSearches)
				if opts.StatusCallback != nil {
					opts.StatusCallback(message)
				} else {
					fmt.Printf("%s\n", message)
				}
			}
		} else {
			// Found useful information, allow normal processing
			finalReq.Tools = a.buildToolsForLLM()
			finalReq.ToolChoice = "auto"
			if enableStreaming {
				message := fmt.Sprintf("Found useful information after %d searches...", searchCount)
				if opts.StatusCallback != nil {
					opts.StatusCallback(message)
				} else {
					fmt.Printf("%s\n", message)
				}
			}
		}
	} else {
		finalReq.Tools = a.buildToolsForLLM()
		finalReq.ToolChoice = "auto"
	}

	// Consistent timeout for all depths to allow persistent searching
	timeout := 45 * time.Second // Fixed timeout regardless of depth
	ctx, cancel := context.WithTimeout(a.ctx, timeout)
	defer cancel()

	finalResp, err := a.llmProvider.Complete(ctx, finalReq)
	if err != nil {
		// If we haven't reached minimum searches, continue trying instead of giving up
		searchCount = a.countSearchAttempts(messages)
		if searchCount < minSearches && depth < maxDepth-2 {
			// Force continuation with a simple response that includes tool calls
			return &llm.CompletionResponse{
				Content: fmt.Sprintf("Continuing search... (%d/%d attempts made)", searchCount, minSearches),
				ToolCalls: []llm.ToolCall{{
					ID:   fmt.Sprintf("retry_search_%d", depth),
					Type: "function",
					Function: llm.FunctionCall{
						Name:      "kbase",
						Arguments: `{"query": "alternative search terms"}`,
					},
				}},
				Usage: llm.TokenUsage{},
			}, nil
		}

		// Extract tool results and return them directly only if we've done enough searches
		var toolResults []string
		for i := len(messages) - 1; i >= 0; i-- {
			if messages[i].Role == "tool" && messages[i].Content != "" {
				toolResults = append(toolResults, messages[i].Content)
			}
		}

		fallbackContent := fmt.Sprintf("After %d search attempts, I'm experiencing technical difficulties. Please try rephrasing your question.", searchCount)
		if len(toolResults) > 0 {
			fallbackContent = fmt.Sprintf("After %d searches, I found some information but am having trouble presenting it properly. Could you please rephrase your question?", searchCount)
		}

		return &llm.CompletionResponse{
			Content: fallbackContent,
			Usage: llm.TokenUsage{
				PromptTokens:     0,
				CompletionTokens: 0,
				TotalTokens:      0,
			},
		}, nil
	}

	// Handle nested tool calls recursively with increased depth
	if len(finalResp.ToolCalls) > 0 && depth < maxDepth-1 {
		if enableStreaming {
			message := fmt.Sprintf("Continuing search (depth %d)...", depth+1)
			if opts.StatusCallback != nil {
				opts.StatusCallback(message)
			} else {
				fmt.Printf("%s\n", message)
			}
		}
		return a.handleToolCallsWithDepthLimitStreaming(finalResp, messages, opts, depth+1, enableStreaming)
	}

	return finalResp, nil
}

// executeToolCall executes a single tool call
func (a *V3Agent) executeToolCall(toolCall llm.ToolCall) (string, error) {
	// Parse function arguments
	var args map[string]interface{}
	if err := json.Unmarshal([]byte(toolCall.Function.Arguments), &args); err != nil {
		return "", fmt.Errorf("failed to parse arguments: %w", err)
	}

	// Execute the tool
	execution := tools.ToolExecution{
		ToolName:   toolCall.Function.Name,
		Parameters: args,
		Confirmed:  true, // Auto-confirm for LLM-requested tools
		Timestamp:  time.Now(),
	}

	result, err := a.toolRegistry.ExecuteTool(a.ctx, execution)
	if err != nil {
		return "", err
	}

	if !result.Success {
		return "", fmt.Errorf("tool execution failed: %s", result.Error)
	}

	// For file_read tool, return the actual content instead of just the success message
	if toolCall.Function.Name == "file_read" && result.Data != nil {
		if dataMap, ok := result.Data.(map[string]interface{}); ok {
			if content, exists := dataMap["content"]; exists {
				if contentStr, ok := content.(string); ok {
					return contentStr, nil
				}
			}
		}
	}

	return result.Message, nil
}

// buildSystemPromptCached creates a dynamic system prompt with caching
func (a *V3Agent) buildSystemPromptCached(hasTools bool) string {
	// Generate cache key based on prompt configuration AND context
	toolCount := 0
	if hasTools {
		toolCount = len(a.toolRegistry.ListAvailableTools(a.ctx))
	}

	// Include context in cache key for proper API host handling
	contextKey := ""
	if a.contextInfo != nil {
		userName := a.contextInfo.UserName
		organization := a.contextInfo.Organization
		apiHost := ""
		if a.contextInfo.Metadata != nil {
			if host, ok := a.contextInfo.Metadata["api_host"]; ok {
				apiHost = fmt.Sprintf("%v", host)
			}
		}
		contextKey = a.promptCache.GenerateContextKey(userName, organization, apiHost)
	}

	cacheKey := a.promptCache.GeneratePromptKey(hasTools, a.toolsOnlyMode, toolCount) + "_" + contextKey

	// Try to get from cache first
	if cached, found := a.promptCache.Get(cacheKey); found {
		return cached
	}

	// Generate new prompt
	prompt := a.buildSystemPrompt(hasTools)

	// Add context information to the prompt if available
	if a.contextInfo != nil {
		contextMessage := a.buildContextMessage(a.contextInfo)
		if contextMessage != "" {
			prompt += "\n\n" + contextMessage
		}
	}

	// Cache the result
	a.promptCache.Set(cacheKey, prompt)

	return prompt
}

// buildSystemPrompt creates a dynamic system prompt that informs the LLM about its capabilities
func (a *V3Agent) buildSystemPrompt(hasTools bool) string {
	var prompt string

	if hasTools {
		toolCount := len(a.toolRegistry.ListAvailableTools(a.ctx))

		// Build prompt from templates
		prompt = prompts.RenderSystemBasePrompt(prompts.PromptData{
			ToolCount: toolCount,
		})

		// Add search strategy
		prompt += "\n\n" + prompts.GetSearchStrategyPrompt()

		// Add filter rules
		prompt += "\n\n" + prompts.GetFilterRulesPrompt()

		// Add anti-hallucination rules
		prompt += "\n\n" + prompts.GetAntiHallucinationPrompt()
	} else {
		// Simple prompt for no-tools mode
		prompt = "You are V3 Agent, a helpful technical assistant."
	}

	// Add tools-only mode if enabled
	if a.toolsOnlyMode {
		prompt += "\n\n" + prompts.GetToolsOnlyModePrompt()
	}

	return prompt
}

// countSearchAttempts counts how many search attempts have been made in the conversation
func (a *V3Agent) countSearchAttempts(messages []llm.Message) int {
	count := 0
	for _, msg := range messages {
		if msg.Role == "assistant" {
			// Count tool calls in assistant messages
			for _, toolCall := range msg.ToolCalls {
				if toolCall.Function.Name == "kbase" {
					count++
				}
			}
		}
	}
	return count
}

// buildContextMessage creates a context message from user information for personalization
func (a *V3Agent) buildContextMessage(ctx *ConversationContext) string {
	if ctx == nil {
		return ""
	}

	var contextParts []string

	// Add user information
	if ctx.UserName != "" {
		contextParts = append(contextParts, fmt.Sprintf("User name: %s", ctx.UserName))
	}

	if ctx.Organization != "" {
		contextParts = append(contextParts, fmt.Sprintf("Organization: %s", ctx.Organization))
	}

	if ctx.Role != "" {
		contextParts = append(contextParts, fmt.Sprintf("Role: %s", ctx.Role))
	}

	// Add preferences
	if len(ctx.Preferences) > 0 {
		var prefs []string
		for key, value := range ctx.Preferences {
			prefs = append(prefs, fmt.Sprintf("%s: %s", key, value))
		}
		contextParts = append(contextParts, fmt.Sprintf("Preferences: %s", strings.Join(prefs, ", ")))
	}

	// Add metadata
	if len(ctx.Metadata) > 0 {
		var metadata []string
		for key, value := range ctx.Metadata {
			metadata = append(metadata, fmt.Sprintf("%s: %v", key, value))
		}
		contextParts = append(contextParts, fmt.Sprintf("Additional context: %s", strings.Join(metadata, ", ")))
	}

	if len(contextParts) == 0 {
		return ""
	}

	// Extract api_host from metadata if available
	apiHost := ""
	if ctx.Metadata != nil {
		if host, ok := ctx.Metadata["api_host"]; ok {
			apiHost = fmt.Sprintf("%v", host)
		}
	}

	hostRule := "🚨 CRITICAL API HOST RULE 🚨\nWhen providing ANY API examples (curl, HTTP requests, URLs):\n"
	if apiHost != "" {
		hostRule += fmt.Sprintf("- ALWAYS use this API host: %s\n", apiHost)
		hostRule += "- NEVER use placeholder hosts like [host], example.com, or api.example.com\n"
		hostRule += "- ALWAYS provide real, executable examples that the user can copy and run immediately"
	}

	return fmt.Sprintf(`CONVERSATION CONTEXT:
%s

Use this information to personalize your responses appropriately. Greet the user naturally using their name and context when appropriate, but don't be overly familiar. Adapt your communication style based on their role and organization when relevant.

%s`, strings.Join(contextParts, "\n"), hostRule)
}
