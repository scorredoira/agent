package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/santiagocorredoira/agent/agent/config"
	"github.com/santiagocorredoira/agent/agent/llm"
	"github.com/santiagocorredoira/agent/agent/memory"
	"github.com/santiagocorredoira/agent/agent/planner"
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
	UserName     string            `json:"user_name,omitempty"`     // User's name for personalization
	Organization string            `json:"organization,omitempty"`  // User's company/club/organization
	Role         string            `json:"role,omitempty"`          // User's role (admin, developer, etc.)
	Preferences  map[string]string `json:"preferences,omitempty"`   // Custom preferences (timezone, language, etc.)
	Metadata     map[string]any    `json:"metadata,omitempty"`      // Additional contextual data
}

// ConversationOptions provides options for conversations
type ConversationOptions struct {
	MaxTokens    int
	Temperature  float64
	SystemPrompt string
	ContextLimit int
	Context      *ConversationContext // Optional context for personalization
}

// DefaultConversationOptions returns sensible defaults
func DefaultConversationOptions() ConversationOptions {
	return ConversationOptions{
		MaxTokens:    2500, // Increased for code examples and detailed explanations
		Temperature:  0.7,
		SystemPrompt: "",
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
	if err := registerBasicTools(toolRegistry, agentConfig); err != nil {
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

	agent := &V3Agent{
		config:        agentConfig,
		llmProvider:   provider,
		memoryManager: memoryManager,
		toolRegistry:  toolRegistry,
		taskPlanner:   taskPlanner,
		ctx:           ctx,
		cancel:        cancel,
		toolsOnlyMode: cfg.ToolsOnlyMode, // Use the configuration value
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

	// Show initial status if streaming enabled
	if enableStreaming {
		fmt.Printf("Processing request...\n")
	}

	// Add context to session if this is the first message and context is available
	if !a.contextAdded && a.contextInfo != nil {
		contextMessage := a.buildContextMessage(a.contextInfo)
		if contextMessage != "" {
			systemMsg := llm.Message{
				Role:    "system", 
				Content: contextMessage,
			}
			if err := a.memoryManager.AddMessageToCurrentSession(systemMsg); err != nil {
				return nil, fmt.Errorf("failed to add context to session: %w", err)
			}
			a.contextAdded = true
		}
	}

	// Add user message to memory
	userMessage := llm.Message{Role: "user", Content: message}
	if err := a.memoryManager.AddMessageToCurrentSession(userMessage); err != nil {
		return nil, fmt.Errorf("failed to add message to session: %w", err)
	}

	// Build tools list for LLM function calling
	if enableStreaming {
		fmt.Printf("Preparing tools...\n")
	}
	availableTools := a.buildToolsForLLM()

	var toolContext string

	// Get contextual messages
	if enableStreaming {
		fmt.Printf("Building context...\n")
	}
	contextMessages := a.memoryManager.GetContextForQuery(message, opts.ContextLimit)

	// Add tool context if available
	if toolContext != "" {
		toolMessage := llm.Message{Role: "system", Content: toolContext}
		contextMessages = append(contextMessages, toolMessage)
	}

	// Add system prompt (combine default agent prompt with user's custom prompt)
	systemPrompt := a.buildSystemPrompt(len(availableTools) > 0)
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
		fmt.Printf("Thinking...\n")
	}
	resp, err := a.llmProvider.Complete(a.ctx, req)
	if err != nil {
		// Simple fallback response instead of failing
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
				fmt.Printf("Searching documentation (%d searches)...\n", len(resp.ToolCalls))
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
	return a.toolRegistry.RegisterTool(tool)
}

// EnableInteractionLogging configures the LLM provider to log all interactions
func (a *V3Agent) EnableInteractionLogging(logger *llm.InteractionLogger, sessionID string) {
	if logger == nil {
		return
	}

	// Wrap the current provider with logging
	a.llmProvider = llm.NewLoggedProvider(a.llmProvider, logger, sessionID)
}

// UnregisterTool removes a tool from the registry
func (a *V3Agent) UnregisterTool(name string) bool {
	return a.toolRegistry.UnregisterTool(name)
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

func registerBasicTools(registry *tools.ToolRegistry, config *config.Config) error {
	// Create restricted filesystem for knowledge base
	restrictedFS, err := tools.NewRestrictedFS(config.KnowledgeBase.Path)
	if err != nil {
		return fmt.Errorf("failed to create restricted filesystem: %w", err)
	}

	// Register the new search engine tool
	searchEngineTool := tools.NewSearchEngineTool(restrictedFS)
	if err := registry.RegisterTool(searchEngineTool); err != nil {
		return fmt.Errorf("failed to register search engine tool: %w", err)
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
	maxDepth := 15    // Allow up to 15 tool calls to find information thoroughly  
	minSearches := 5  // Minimum searches required before giving up (more reasonable)

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
			fmt.Printf("Searching %d/%d...\n", i+1, len(initialResp.ToolCalls))
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
		fmt.Printf("Processing results...\n")
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
			fmt.Printf("Max search depth reached...\n")
		}
	} else if searchCount < minSearches {
		// Check if recent searches found useful information (be stricter)
		recentToolResults := 0
		recentToolContent := ""
		for i := len(messages) - 1; i >= 0 && i >= len(messages)-2; i-- {
			if messages[i].Role == "tool" && len(messages[i].Content) > 500 {
				content := strings.ToLower(messages[i].Content)
				// Only consider it useful if it contains specific technical information
				if (strings.Contains(content, "http") || strings.Contains(content, "post") || 
				    strings.Contains(content, "get") || strings.Contains(content, "endpoint")) &&
				   (strings.Contains(content, "json") || strings.Contains(content, "parameter") ||
				    strings.Contains(content, "request") || strings.Contains(content, "response")) {
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
					// Add a system message to force more searching
					forceSearchMsg := llm.Message{
						Role:    "system",
						Content: fmt.Sprintf("🚨 SEARCH REQUIREMENT 🚨\nYou have made %d/%d searches. Continue searching until you find useful information or reach %d total searches.\n\nSEARCH STRATEGIES:\n- Different keywords and synonyms\n- Technical variations\n- Related concepts\n\nIf you found useful information in recent searches, you may provide an answer. Otherwise, use the kbase tool with different search terms.", searchCount, minSearches, minSearches),
					}
					messages = append(messages, forceSearchMsg)
					finalReq.Messages = messages
				}
			}
			if enableStreaming {
				fmt.Printf("Forcing more searches (%d/%d)...\n", searchCount, minSearches)
			}
		} else {
			// Found useful information, allow normal processing
			finalReq.Tools = a.buildToolsForLLM()
			finalReq.ToolChoice = "auto"
			if enableStreaming {
				fmt.Printf("Found useful information after %d searches...\n", searchCount)
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
			fmt.Printf("Continuing search (depth %d)...\n", depth+1)
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

// buildSystemPrompt creates a dynamic system prompt that informs the LLM about its capabilities
func (a *V3Agent) buildSystemPrompt(hasTools bool) string {
	prompt := `You are V3 Agent, a technical assistant. You execute tool calls IMMEDIATELY when needed.`

	if hasTools {
		toolCount := len(a.toolRegistry.ListAvailableTools(a.ctx))
		prompt += fmt.Sprintf(`

🔧 CRITICAL: You have %d tools available. Use them ONLY when you need specific technical information.

WHEN TO USE TOOLS (search documentation):
- User asks about API endpoints, parameters, or syntax
- User needs specific technical details about features
- User asks how to implement something specific
- User needs code examples or configuration details
- User asks about data models, fields, or database structure

WHEN NOT TO USE TOOLS (respond directly):
- Basic greetings: "hello", "hi", "hola", "buenos días" (use context to personalize if available)
- Time/Date queries: "what time is it", "what's the date", "qué hora es"
- Simple status: "how are you", "cómo estás"
- Basic courtesy: "thanks", "gracias", "goodbye", "adiós"
- Simple clarifications: "yes/no questions", "did you understand"

EXAMPLES:
✅ USE TOOLS: "how do I create a customer", "what's the paySales endpoint"
❌ DON'T USE TOOLS: "hello", "what time is it", "thanks"

EXECUTION RULES WHEN USING TOOLS:
- MINIMUM 10 SEARCHES required before admitting information not found
- If first searches fail → Try different keywords, synonyms, technical terms
- NEVER stop searching until you've exhausted all possible approaches
- When asked for code examples → SEARCH documentation FIRST, then provide exact examples  
- When information is missing → STATE EXACTLY what is missing and suggest next steps
- BE PERSISTENT: Try up to 20 different approaches if needed

🚨 LANGUAGE RULE: ALL SEARCHES MUST BE IN ENGLISH 🚨
- The documentation is ONLY available in English
- ALWAYS search using English terms, even if the user asks in another language
- Example: User asks "leer clientes" → Search for "read customers", "get customers", "list customers"
- NEVER search in Spanish, Portuguese, French, or any other language


COMMUNICATION STYLE:
- Be direct and factual
- Present information naturally without exposing search mechanics
- NEVER reveal tool names, scores, or internal search details
- NEVER show "Found X files" or "Score: X.XX" or "Reason: ..."
- State exactly what you found or what is missing
- Provide concrete next steps when information is unavailable
- Reference documentation naturally: "According to the documentation" or "The API documentation shows"

🔍 PROGRESSIVE SEARCH STRATEGY:
When users ask for technical information:
1. FIRST SEARCH: Use exact user terms translated to English
2. SECOND SEARCH: Try direct API operation names (e.g., "paySales", "saveSale")
3. THIRD SEARCH: Try broader conceptual terms
4. FOURTH SEARCH: Try with "endpoint", "API" suffixes
5. FIFTH+ SEARCHES: Try variations, synonyms, related terms

SPECIFIC SEARCH EXAMPLES:
- User asks "pagar ventas" → Search: "pay sales", "paySales", "payment sales", "billing pay"
- User asks "leer clientes" → Search: "read customers", "get customers", "customer endpoint", "list customers"
- User asks "crear factura" → Search: "create invoice", "invoice endpoint", "new invoice", "save invoice"
- User asks "cancelar booking" → Search: "cancel booking", "booking cancel", "delete booking"

SEARCH DEPTH REQUIREMENTS:
- MINIMUM 10 DIFFERENT search terms before admitting failure
- Each search must use meaningfully different keywords
- Try compound terms: "billing paySales", "customer get", "invoice create"
- Try action verbs: "pay", "create", "read", "update", "delete", "list", "get"

CRITICAL ENDPOINT PATTERNS TO LOOK FOR:
- "GET https://[host]/api/model/ENTITY" = List endpoint
- "GET https://[host]/api/model/ENTITY/[id]" = Single item endpoint
- "POST https://[host]/api/model/ENTITY" = Create endpoint

EXAMPLE: If search results show Content containing:
"GET https://[host]/api/model/payment/[id]" AND 
"GET https://[host]/api/model/payment"
→ The list endpoint is: GET https://[host]/api/model/payment

DOCUMENTED API FACTS (use these ONLY):
- Authentication: headers "key" and "tenant" 
- Pagination: "lastId", "limit", "search" parameters
- Filtering: search parameter with format ["field", "operator", "value"]
- Field selection: fields parameter
- No orderBy parameters exist
- No Authorization headers exist
- No Bearer tokens exist`, toolCount)
	}

	prompt += `

STRICT PROHIBITIONS - NEVER DO THESE:
- NEVER say "I apologize" or similar phrases
- NEVER promise to search without executing the search
- NEVER provide generic advice without documentation backing
- NEVER make up code examples or API endpoints
- NEVER invent authentication methods, headers, or parameters
- NEVER use standard conventions like Bearer tokens, Authorization headers, or orderBy unless documented
- NEVER assume anything about API behavior
- If you cannot find exact parameters in documentation, say so explicitly and suggest alternatives

🚨 ABSOLUTE ANTI-HALLUCINATION RULES 🚨
- NEVER invent operators like "startsWith", "contains", "like", "endsWith" without explicit documentation
- NEVER suggest syntax patterns not found in actual documentation
- NEVER assume common programming conventions apply unless documented
- NEVER describe features, parameters, or behaviors not explicitly found in documentation
- NEVER say "according to documentation" unless you actually found and can cite specific documentation
- NEVER invent status codes, field meanings, or API behaviors
- If you don't find specific documentation, say exactly: "I could not find documentation for [specific thing]"

🚨 FORBIDDEN PHRASES 🚨
NEVER use these phrases unless you have explicit documentation:
- "According to the documentation"
- "The system automatically"
- "This suggests that"
- "Typically this means"
- "Usually the process is"
- "The correct way would be"

ONLY USE THESE SAFE PHRASES:
- "I found this endpoint in the documentation:"
- "The documentation shows:"
- "I could not find documentation for:"
- "No information found about:"

🚨 CRITICAL ENDPOINT RULE 🚨
NEVER INVENT ENDPOINTS LIKE:
- /api/v2/anything
- /api/v1/anything  
- Any endpoint not found in documentation
- Any URL patterns not explicitly documented

If no endpoint is documented for the requested operation, respond EXACTLY:
"I could not find a documented endpoint for [operation]. After searching the documentation, I found these related endpoints: [list actual findings]"

CRITICAL: Only use documented information. When in doubt, search documentation first.`

	// Add ToolsOnlyMode enforcement if enabled
	if a.toolsOnlyMode {
		prompt += `

🚨 TOOLS-ONLY MODE ACTIVE 🚨
You MUST ONLY respond to questions that require using tools, EXCEPT for basic interactions and greetings.

ALLOWED BASIC INTERACTIONS (respond directly, keep brief):
- Greetings: "hello", "hi", "hola", "buenos días" → Simple greeting, offer technical help
- Time/Date: "what time is it", "what's the date", "qué hora es" → Current time/date
- Simple status: "how are you", "cómo estás" → Brief response, redirect to technical help
- Basic courtesy: "thanks", "gracias", "goodbye", "adiós" → Acknowledge politely

TECHNICAL QUESTIONS (use tools):
- API questions: "how do I create a customer" → Search documentation
- Implementation: "what's the paySales endpoint" → Search documentation  
- Code examples: "show me billing API" → Search documentation

REFUSE EXTENDED CONVERSATIONS:
- Jokes, stories, personal opinions
- Lengthy explanations of basic concepts
- Open-ended discussions
- Non-technical advice

For refused topics, respond with:
"I'm configured to assist with technical questions and basic interactions. Please ask me about API endpoints, implementation details, or specific technical features."

EXAMPLES:
✅ ALLOW: "hello" → "Hi! How can I help you with technical questions today?"
✅ ALLOW: "what time is it" → "It's [current time]. Need help with any technical questions?"
✅ ALLOW: "how do I pay a sale" → [Search documentation]
❌ REFUSE: "tell me a joke" → Use refusal message
❌ REFUSE: "explain databases in general" → Use refusal message`
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

	return fmt.Sprintf(`CONVERSATION CONTEXT:
%s

Use this information to personalize your responses appropriately. Greet the user naturally using their name and context when appropriate, but don't be overly familiar. Adapt your communication style based on their role and organization when relevant.`, strings.Join(contextParts, "\n"))
}
