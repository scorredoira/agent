package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/santiagocorredoira/agent/agent"
	"github.com/santiagocorredoira/agent/agent/llm"
	"github.com/santiagocorredoira/agent/agent/memory"
	"golang.org/x/term"
)

const (
	version = "0.2.0"
	banner  = `
██╗   ██╗██████╗      █████╗  ██████╗ ███████╗███╗   ██╗████████╗
██║   ██║╚════██╗    ██╔══██╗██╔════╝ ██╔════╝████╗  ██║╚══██╔══╝
██║   ██║ █████╔╝    ███████║██║  ███╗█████╗  ██╔██╗ ██║   ██║   
╚██╗ ██╔╝ ╚═══██╗    ██╔══██║██║   ██║██╔══╝  ██║╚██╗██║   ██║   
 ╚████╔╝ ██████╔╝    ██║  ██║╚██████╔╝███████╗██║ ╚████║   ██║   
  ╚═══╝  ╚═════╝     ╚═╝  ╚═╝ ╚═════╝ ╚══════╝╚═╝  ╚═══╝   ╚═╝   
                                                                  
    🚀 Advanced AI Assistant with Semantic Search & Function Calling
       💬 Just start chatting or type 'help' for commands
`
)

// LogLevel represents different levels of logging verbosity
type LogLevel int

const (
	LogLevelNormal  LogLevel = iota // Minimal output
	LogLevelVerbose                 // Additional info
	LogLevelDebug                   // Full debugging info
)

// Global log level
var logLevel LogLevel = LogLevelNormal

// Logging functions
func logNormal(format string, args ...interface{}) {
	if logLevel >= LogLevelNormal {
		fmt.Printf(format, args...)
	}
}

func logVerbose(format string, args ...interface{}) {
	if logLevel >= LogLevelVerbose {
		fmt.Printf("🔍 "+format, args...)
	}
}

func logDebug(format string, args ...interface{}) {
	if logLevel >= LogLevelDebug {
		fmt.Printf("🐛 "+format, args...)
	}
}

// cleanContent removes control characters and fixes formatting
func cleanContent(content string) string {
	// Remove carriage returns and clean up whitespace
	content = strings.ReplaceAll(content, "\r", "")

	// Remove excessive newlines
	for strings.Contains(content, "\n\n\n") {
		content = strings.ReplaceAll(content, "\n\n\n", "\n\n")
	}

	// Remove leading/trailing whitespace
	content = strings.TrimSpace(content)

	return content
}

// CLI represents the command-line interface
type CLI struct {
	agent       *agent.V3Agent
	ctx         context.Context
	cancel      context.CancelFunc
	logger      *llm.InteractionLogger
	sessionID   string
	interactive bool // Track if we're in interactive mode
	context     *agent.ConversationContext // User context for personalization
}

// NewCLI creates a new CLI instance
func NewCLI(configPath string, enableInteractionLog bool, logDir string, interactive bool, convContext *agent.ConversationContext) (*CLI, error) {
	stepStart := time.Now()

	// Use shared storage directory for consistency with web interface
	sharedStorageDir := "./agent_memory"

	config := agent.AgentConfig{
		ConfigPath:    configPath,
		StorageDir:    sharedStorageDir,
		ToolsOnlyMode: true, // Default: only respond to technical questions requiring tools
	}
	logVerbose("Config loaded (%.3fs)\n", time.Since(stepStart).Seconds())

	// Configure interaction logging if enabled using shared directory
	var logger *llm.InteractionLogger
	if enableInteractionLog {
		loggerConfig := &llm.LoggerConfig{
			Enabled:     true,
			LogDir:      logDir, // Use shared log directory
			MaxSessions: 50,
		}
		logger = llm.NewInteractionLogger(loggerConfig)
		logVerbose("Interaction logging enabled: %s\n", logDir)
	}

	// Create V3 Agent
	stepStart = time.Now()
	v3agent, err := agent.NewV3Agent(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create V3 agent: %w", err)
	}
	logVerbose("V3 Agent created (%.3fs)\n", time.Since(stepStart).Seconds())

	// Set up logging if enabled
	if logger != nil {
		// We'll set up the logged provider after creating the session
		logVerbose("Logged provider will be configured with session\n")
	}

	// Create context with cancellation
	ctx, cancel := context.WithCancel(context.Background())

	// Generate unique session ID for this CLI session
	sessionID := fmt.Sprintf("cli_%d", time.Now().UnixNano())

	// Start logging session if enabled
	if logger != nil {
		metadata := map[string]interface{}{
			"type":    "cli_session",
			"version": version,
			"config":  configPath,
		}
		logger.StartSession(sessionID, metadata)
		logVerbose("Started logging session: %s\n", sessionID)

		// Enable interaction logging on the agent
		v3agent.EnableInteractionLogging(logger, sessionID)
		logVerbose("Interaction logging enabled on agent\n")
	}

	cli := &CLI{
		agent:       v3agent,
		ctx:         ctx,
		cancel:      cancel,
		logger:      logger,
		sessionID:   sessionID,
		interactive: interactive,
		context:     convContext,
	}

	return cli, nil
}

// runSingleQuery processes a single query and exits
func runSingleQuery(query, configPath string, enableInteractionLog bool, logDir string, contextFile string) {
	// Load conversation context if provided or auto-detect context.json
	contextPath := contextFile
	if contextPath == "" && fileExists("context.json") {
		contextPath = "context.json"
	}
	
	convContext, err := loadContextFromFile(contextPath)
	if err != nil {
		fmt.Printf("❌ Failed to load context: %v\n", err)
		os.Exit(1)
	}
	
	// Create CLI for single query (not interactive)
	cli, err := NewCLI(configPath, enableInteractionLog, logDir, false, convContext)
	if err != nil {
		fmt.Printf("❌ Failed to initialize: %v\n", err)
		os.Exit(1)
	}
	defer cli.Shutdown()

	// Start conversation session with context if available  
	if convContext != nil {
		_, err = cli.agent.StartConversationWithContext(convContext)
	} else {
		_, err = cli.agent.StartConversation()
	}
	if err != nil {
		fmt.Printf("❌ Failed to start session: %v\n", err)
		os.Exit(1)
	}

	// Process the query
	resp, _, err := cli.completeWithProgress(query)
	if err != nil {
		fmt.Printf("❌ Error: %v\n", err)
		os.Exit(1)
	}

	// Output the response
	cleanedContent := cleanContent(resp.Content)
	fmt.Println(cleanedContent)
}

func main() {
	// Parse command line flags
	var verbose = flag.Bool("v", false, "Enable verbose logging")
	var debug = flag.Bool("debug", false, "Enable debug logging (includes LLM traffic)")
	var showVersion = flag.Bool("version", false, "Show version and exit")
	var configPath = flag.String("config", "config.json", "Path to configuration file")
	var enableInteractionLog = flag.Bool("log-interactions", true, "Enable LLM interaction logging (enabled by default)")
	var disableLogs = flag.Bool("no-logs", false, "Disable all interaction logging")
	var logDir = flag.String("log-dir", "./logs", "Directory for interaction logs")
	var interactive = flag.Bool("interactive", false, "Force interactive mode even when query is provided")
	var contextFile = flag.String("context", "", "Path to context JSON file for personalization")
	flag.Parse()

	// Handle version flag
	if *showVersion {
		fmt.Printf("V3 Agent version %s\n", version)
		os.Exit(0)
	}

	// Set log level based on flags
	if *debug {
		logLevel = LogLevelDebug
	} else if *verbose {
		logLevel = LogLevelVerbose
	}

	start := time.Now()

	// Check if we have a query argument and interactive mode
	args := flag.Args()
	if len(args) > 0 && !*interactive {
		// Single query mode - process query and exit
		query := strings.Join(args, " ")
		runSingleQuery(query, *configPath, *enableInteractionLog && !*disableLogs, *logDir, *contextFile)
		return
	}

	// Interactive mode
	fmt.Print(banner)

	// Load conversation context if provided or auto-detect context.json
	contextPath := *contextFile
	if contextPath == "" && fileExists("context.json") {
		contextPath = "context.json"
		logVerbose("Auto-detected context.json\n")
	}
	
	convContext, err := loadContextFromFile(contextPath)
	if err != nil {
		fmt.Printf("❌ Failed to load context: %v\n", err)
		os.Exit(1)
	}
	
	// Debug: show context info in verbose mode
	if convContext != nil {
		logVerbose("Context loaded: User=%s, Org=%s\n", convContext.UserName, convContext.Organization)
	}
	
	// Create CLI
	logVerbose("Initializing V3 Agent...\n")
	finalLogEnabled := *enableInteractionLog && !*disableLogs
	cli, err := NewCLI(*configPath, finalLogEnabled, *logDir, true, convContext)
	if err != nil {
		fmt.Printf("❌ Failed to initialize CLI: %v\n", err)
		os.Exit(1)
	}
	defer cli.Shutdown()

	logVerbose("V3 Agent initialized (%.2fs)\n", time.Since(start).Seconds())

	// Setup signal handling
	setupSignalHandling(cli)

	// Start interactive session, optionally with initial query
	var initialQuery string
	if len(args) > 0 && *interactive {
		initialQuery = strings.Join(args, " ")
	}

	if err := cli.StartInteractiveSession(initialQuery); err != nil {
		fmt.Printf("❌ Session error: %v\n", err)
		os.Exit(1)
	}
}

func (c *CLI) StartInteractiveSession(initialQuery string) error {
	// Start new conversation session with context if available
	var session *memory.ConversationMemory
	var err error
	
	if c.context != nil {
		logVerbose("Starting session with context for user: %s\n", c.context.UserName)
		session, err = c.agent.StartConversationWithContext(c.context)
	} else {
		session, err = c.agent.StartConversation()
	}
	
	if err != nil {
		return fmt.Errorf("failed to start session: %w", err)
	}

	stats := c.agent.GetStats()
	logVerbose("Session started: %s\n", session.SessionID)
	logVerbose("Available tools: %d\n", stats.AvailableTools)
	logVerbose("Memory system: Active\n")
	logVerbose("%s\n", strings.Repeat("─", 60))

	// Show automatic greeting
	c.showSimpleGreeting()

	// Process initial query if provided
	if initialQuery != "" {
		logNormal("Processing initial query: %s\n\n", initialQuery)
		if err := c.processUserMessage(initialQuery); err != nil {
			fmt.Printf("❌ Error processing initial query: %v\n", err)
		}
		fmt.Println() // Add spacing before interactive prompt
	}

	// Main conversation loop
	scanner := bufio.NewScanner(os.Stdin)
	for {
		if logLevel >= LogLevelVerbose {
			fmt.Print("\n🤖 You: ")
		} else {
			fmt.Print("\nYou: ")
		}

		if !scanner.Scan() {
			break
		}

		input := strings.TrimSpace(scanner.Text())
		if input == "" {
			continue
		}

		// Handle special commands
		if handled := c.handleCommand(input); handled {
			continue
		}

		// Process user message
		if err := c.processUserMessage(input); err != nil {
			fmt.Printf("❌ Error processing message: %v\n", err)
		}
	}

	return scanner.Err()
}

func (c *CLI) processUserMessage(input string) error {
	logDebug("User input: %s\n", input)

	// Send message to agent with progress indicator
	resp, duration, err := c.completeWithProgress(input)
	if err != nil {
		// Don't fail the CLI, show error but continue
		logNormal("⚠️  Error: %v\n", err)
		logNormal("💡 Please try again or rephrase your question.\n")
		return nil // Continue CLI instead of failing
	}

	// Clean the response content
	cleanedContent := cleanContent(resp.Content)

	logDebug("LLM Response: %s\n", cleanedContent)
	if len(resp.ToolCalls) > 0 {
		logDebug("Tool calls requested: %d\n", len(resp.ToolCalls))
		for i, tc := range resp.ToolCalls {
			logDebug("  Tool %d: %s(%s)\n", i+1, tc.Function.Name, tc.Function.Arguments)
		}
	}

	if logLevel >= LogLevelVerbose {
		logNormal("🧠 Agent: %s\n", cleanedContent)
	} else {
		logNormal("Agent: %s\n", cleanedContent)
	}

	// Show timing info
	stats := c.agent.GetStats()
	if logLevel >= LogLevelVerbose {
		logVerbose("Response: %d tokens, %.2fs, %s\n",
			resp.Usage.TotalTokens, duration.Seconds(), stats.LLMProvider)
	}

	return nil
}

func (c *CLI) handleCommand(input string) bool {
	args := strings.Fields(input)
	if len(args) == 0 {
		return false
	}

	command := strings.ToLower(args[0])

	switch command {
	case "help", "/help":
		c.showHelp()
		return true

	case "tools", "/tools":
		c.showTools()
		return true

	case "stats", "/stats":
		c.showStats()
		return true

	case "memory", "/memory":
		c.showMemoryInfo()
		return true

	case "sessions", "/sessions":
		c.showSessions()
		return true

	case "clear", "/clear":
		c.clearScreen()
		return true

	case "config", "/config":
		c.showConfig()
		return true

	case "exit", "/exit", "quit", "/quit":
		fmt.Println("👋 Goodbye!")
		c.Shutdown()
		os.Exit(0)
		return true

	case "version", "/version":
		fmt.Printf("V3 Agent CLI Version: %s\n", version)
		return true

	default:
		return false
	}
}

func (c *CLI) showHelp() {
	fmt.Println("\n📖 Available Commands:")
	fmt.Println("═════════════════════")
	fmt.Println("help       - Show this help message")
	fmt.Println("tools      - List available tools")
	fmt.Println("stats      - Show system statistics")
	fmt.Println("memory     - Show memory information")
	fmt.Println("sessions   - List conversation sessions")
	fmt.Println("config     - Show current configuration")
	fmt.Println("clear      - Clear screen")
	fmt.Println("version    - Show version information")
	fmt.Println("exit/quit  - Exit the agent")
	fmt.Println("\n💬 Or simply type your message to chat with V3 Agent!")
}

func (c *CLI) showTools() {
	fmt.Println("\n🔧 Available Tools:")
	fmt.Println("═══════════════════")

	availableTools := c.agent.GetAvailableTools()
	toolsByCategory := make(map[string][]string)

	for _, tool := range availableTools {
		category := string(tool.Category)
		toolInfo := fmt.Sprintf("%s - %s", tool.Name, tool.Description)
		if tool.RequiresConfirmation {
			toolInfo += " ⚠️"
		}
		toolsByCategory[category] = append(toolsByCategory[category], toolInfo)
	}

	for category, categoryTools := range toolsByCategory {
		fmt.Printf("\n📁 %s:\n", strings.ToUpper(category))
		for _, tool := range categoryTools {
			fmt.Printf("   • %s\n", tool)
		}
	}

	fmt.Printf("\nTotal: %d tools available\n", len(availableTools))
}

func (c *CLI) showStats() {
	fmt.Println("\n📊 System Statistics:")
	fmt.Println("════════════════════")

	stats := c.agent.GetStats()

	// Memory stats
	fmt.Printf("🧠 Memory:\n")
	fmt.Printf("   Sessions: %d\n", stats.TotalSessions)
	fmt.Printf("   Current messages: %d\n", stats.CurrentMessages)
	fmt.Printf("   Context providers: %d\n", stats.ContextProviders)

	// Tool stats
	fmt.Printf("\n🔧 Tools:\n")
	fmt.Printf("   Total: %d (Available: %d)\n", stats.TotalTools, stats.AvailableTools)
	fmt.Printf("   Executions: %d\n", stats.ToolExecutions)
	fmt.Printf("   Success rate: %.1f%%\n", stats.ToolSuccessRate*100)

	// LLM provider info
	fmt.Printf("\n🤖 LLM Provider:\n")
	fmt.Printf("   Provider: %s\n", stats.LLMProvider)
	fmt.Printf("   Available: %t\n", stats.LLMAvailable)
}

func (c *CLI) showMemoryInfo() {
	currentSession := c.agent.GetCurrentSession()
	if currentSession == nil {
		fmt.Println("❌ No active session")
		return
	}

	fmt.Println("\n🧠 Memory Information:")
	fmt.Println("═════════════════════")
	fmt.Printf("Session ID: %s\n", currentSession.SessionID)
	fmt.Printf("Messages: %d\n", len(currentSession.Messages))
	fmt.Printf("Topics: %v\n", currentSession.Topics)
	fmt.Printf("Key facts: %d\n", len(currentSession.KeyFacts))

	if len(currentSession.KeyFacts) > 0 {
		fmt.Println("\nKey Facts:")
		for i, fact := range currentSession.KeyFacts {
			if i >= 3 { // Show only first 3
				fmt.Printf("   ... and %d more\n", len(currentSession.KeyFacts)-3)
				break
			}
			fmt.Printf("   • %s\n", fact.Content)
		}
	}

	stats := c.agent.GetStats()
	fmt.Printf("\nContext Providers: %d\n", stats.ContextProviders)
}

func (c *CLI) showSessions() {
	fmt.Println("\n📚 Conversation Sessions:")
	fmt.Println("═════════════════════════")

	sessions, err := c.agent.ListConversations()
	if err != nil {
		fmt.Printf("❌ Error listing sessions: %v\n", err)
		return
	}

	if len(sessions) == 0 {
		fmt.Println("No sessions found.")
		return
	}

	currentSession := c.agent.GetCurrentSession()
	for i, session := range sessions {
		current := ""
		if currentSession != nil && session.SessionID == currentSession.SessionID {
			current = " ← current"
		}
		fmt.Printf("%d. %s%s\n", i+1, session.SessionID, current)
		fmt.Printf("   Start: %s, Duration: %s\n",
			session.StartTime.Format("2006-01-02 15:04"), session.Duration)
		fmt.Printf("   Messages: %d, Topics: %v\n", session.MessageCount, session.Topics)
		if session.Summary != "" {
			fmt.Printf("   Summary: %.80s...\n", session.Summary)
		}
		fmt.Println()
	}
}

func (c *CLI) showConfig() {
	fmt.Println("\n⚙️ Configuration:")
	fmt.Println("═══════════════════")
	stats := c.agent.GetStats()
	fmt.Printf("LLM Provider: %s\n", stats.LLMProvider)
	fmt.Printf("Available: %t\n", stats.LLMAvailable)
	fmt.Printf("Tools: %d\n", stats.AvailableTools)
	fmt.Printf("Sessions: %d\n", stats.TotalSessions)
}

func (c *CLI) clearScreen() {
	fmt.Print("\033[2J\033[H")
	fmt.Print(banner)
}

func (c *CLI) Shutdown() {
	logVerbose("Shutting down V3 Agent...\n")

	// End logging session if enabled
	if c.logger != nil {
		if err := c.logger.EndSession(c.sessionID); err != nil {
			logVerbose("Warning: Failed to end logging session: %v\n", err)
		} else {
			logVerbose("Logging session saved: %s\n", c.sessionID)
		}
	}

	// Cancel context to stop any ongoing operations
	c.cancel()

	// Ensure terminal is properly restored (only for interactive mode)
	if c.interactive && term.IsTerminal(int(os.Stdin.Fd())) {
		// Reset terminal to normal mode only in interactive mode
		fmt.Print("\033[?1049l") // Exit alternate screen buffer if used
		fmt.Print("\033[0m")     // Reset all formatting
	}

	// Save current session if exists
	currentSession := c.agent.GetCurrentSession()
	if currentSession != nil {
		if err := currentSession.Save(); err != nil {
			fmt.Printf("⚠️ Warning: Failed to save session: %v\n", err)
		}
	}

	logVerbose("V3 Agent shutdown complete\n")
}

// Helper functions

// loadContextFromFile loads conversation context from a JSON file
func loadContextFromFile(filePath string) (*agent.ConversationContext, error) {
	if filePath == "" {
		return nil, nil
	}

	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read context file %s: %w", filePath, err)
	}

	var context agent.ConversationContext
	if err := json.Unmarshal(data, &context); err != nil {
		return nil, fmt.Errorf("failed to parse context file %s: %w", filePath, err)
	}

	return &context, nil
}

// fileExists checks if a file exists
func fileExists(filename string) bool {
	_, err := os.Stat(filename)
	return err == nil
}

// showSimpleGreeting shows a simple automatic greeting based on context
func (c *CLI) showSimpleGreeting() {
	fmt.Println() // Add blank line before greeting
	if c.context != nil && c.context.UserName != "" {
		logNormal("Agent: Hello %s! How can I help you today?\n", c.context.UserName)
	} else {
		logNormal("Agent: Hello! How can I help you today?\n")
	}
}

func setupSignalHandling(cli *CLI) {
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-c
		fmt.Printf("\n\n🛑 Received interrupt signal\n")
		cli.Shutdown()
		os.Exit(0)
	}()
}

// completeWithProgress shows a thinking indicator while waiting for LLM response
func (c *CLI) completeWithProgress(input string) (*llm.CompletionResponse, time.Duration, error) {
	// Channel for LLM response
	respChan := make(chan *llm.CompletionResponse, 1)
	errChan := make(chan error, 1)

	// Channel for user cancellation
	cancelChan := make(chan bool, 1)

	// Context for cancellation
	_, cancel := context.WithCancel(c.ctx)
	defer cancel()

	start := time.Now()

	// Start LLM request in goroutine
	go func() {
		// Log request details in debug mode
		if logLevel >= LogLevelDebug {
			logDebug("Sending message to LLM: %s\n", input)
		}

		resp, err := c.agent.SendMessageWithStreaming(input)
		if err != nil {
			logDebug("LLM error: %v\n", err)
			errChan <- err
		} else {
			logDebug("LLM response received (%d tokens)\n", resp.Usage.TotalTokens)
			respChan <- resp
		}
	}()

	// Start keyboard monitoring in goroutine (only in verbose mode to avoid terminal issues)
	if logLevel >= LogLevelVerbose {
		go c.monitorKeyboard(cancelChan)
	}

	// Show thinking indicator (disabled when using streaming)
	// go c.showThinkingIndicator(start, cancelChan)

	// Wait for response or cancellation
	select {
	case resp := <-respChan:
		cancelChan <- true    // Stop indicator
		fmt.Print("\r\033[K") // Clear the thinking line
		return resp, time.Since(start), nil

	case err := <-errChan:
		cancelChan <- true    // Stop indicator
		fmt.Print("\r\033[K") // Clear the thinking line
		return nil, time.Since(start), err

	case <-cancelChan:
		cancel()              // Cancel the LLM request
		fmt.Print("\r\033[K") // Clear the thinking line
		fmt.Println("🚫 Request cancelled by user")
		return nil, time.Since(start), fmt.Errorf("request cancelled by user")
	}
}

// showThinkingIndicator displays animated thinking indicator with adaptive messaging
func (c *CLI) showThinkingIndicator(start time.Time, cancel <-chan bool) {
	indicators := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	i := 0
	for {
		select {
		case <-cancel:
			return
		case <-ticker.C:
			elapsed := time.Since(start)

			// Adaptive messaging based on elapsed time
			var message string
			switch {
			case elapsed < 5*time.Second:
				message = "Thinking…"
			case elapsed < 15*time.Second:
				message = "Processing with tools…"
			case elapsed < 30*time.Second:
				message = "Trying alternative approach…"
			case elapsed < 45*time.Second:
				message = "Using fallback strategy…"
			default:
				message = "Working hard on your request…"
			}

			fmt.Printf("\r%s %s (%.1fs · esc to interrupt)",
				indicators[i%len(indicators)], message, elapsed.Seconds())
			i++
		}
	}
}

// monitorKeyboard watches for ESC key to cancel request
func (c *CLI) monitorKeyboard(cancel chan<- bool) {
	// Set terminal to raw mode to catch individual keystrokes
	oldState, err := term.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		return // Can't monitor keyboard, continue without cancellation
	}

	// Ensure terminal is always restored
	defer func() {
		term.Restore(int(os.Stdin.Fd()), oldState)
	}()

	buf := make([]byte, 1)
	for {
		// Read with a timeout to periodically check if we should exit
		n, err := os.Stdin.Read(buf)
		if err != nil {
			// If we get an error, just exit gracefully
			return
		}
		if n == 0 {
			continue
		}

		// Check for ESC key (ASCII 27)
		if buf[0] == 27 {
			select {
			case cancel <- true:
			default:
				// If can't send to cancel channel, just return
			}
			return
		}
	}
}
