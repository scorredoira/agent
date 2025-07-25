package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"github.com/santiagocorredoira/agent/agent"
	"github.com/santiagocorredoira/agent/agent/llm"
)

func main() {
	// Command line flags
	port := flag.String("port", "8080", "Port to run the server on")
	configPath := flag.String("config", "", "Path to config file")
	storageDir := flag.String("storage", "./agent_memory", "Directory for storing conversation data")
	logDir := flag.String("logs", "./logs", "Directory for storing interaction logs")
	enableLogs := flag.Bool("enable-logs", true, "Enable interaction logging")
	flag.Parse()

	// Auto-detect config file location
	if *configPath == "" {
		// Try different locations depending on where we're running from
		candidates := []string{
			"./config.json",     // Running from project root
			"../../config.json", // Running from cmd/chat directory
		}

		for _, candidate := range candidates {
			if _, err := os.Stat(candidate); err == nil {
				*configPath = candidate
				log.Printf("Found config file: %s", candidate)
				break
			}
		}

		if *configPath == "" {
			log.Println("No config file found, using defaults")
			*configPath = "config.json" // Will use defaults
		}
	}

	// Create agent configuration
	agentConfig := agent.AgentConfig{
		ConfigPath:    *configPath,
		StorageDir:    *storageDir,
		AutoSave:      true,
		ToolsOnlyMode: false, // Allow general conversation for chat interface
	}

	// Configure interaction logging
	var logger *llm.InteractionLogger
	if *enableLogs {
		loggerConfig := &llm.LoggerConfig{
			Enabled:     true,
			LogDir:      *logDir,
			MaxSessions: 100,
		}
		logger = llm.NewInteractionLogger(loggerConfig)
		log.Printf("Interaction logging enabled: %s", *logDir)
	}

	// Initialize the agent
	log.Println("Initializing agent...")
	agentInstance, err := agent.NewV3Agent(agentConfig)
	if err != nil {
		log.Fatalf("Failed to create agent: %v", err)
	}
	defer agentInstance.Shutdown()

	// Set up logging if enabled
	if logger != nil {
		agentInstance.SetLogger(logger)
		log.Println("Interaction logger configured for agent")
	}

	// Wait for agent to be fully initialized
	log.Println("Waiting for LLM providers to initialize...")
	if err := agentInstance.WaitForReady(30 * time.Second); err != nil {
		log.Fatalf("Agent failed to initialize: %v", err)
	}

	stats := agentInstance.GetStats()
	log.Printf("✅ Agent ready - LLM Provider: %s, Available: %t", stats.LLMProvider, stats.LLMAvailable)

	// Set up HTTP routes
	http.HandleFunc("/ws", agentInstance.ServeWebSocket)
	
	// Chat configuration endpoint
	http.HandleFunc("/api/config", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		title := "V3 Agent Chat" // Default title
		if agentInstance.GetConfig().Chat.Title != "" {
			title = agentInstance.GetConfig().Chat.Title
		}
		
		config := map[string]interface{}{
			"title": title,
		}
		
		json.NewEncoder(w).Encode(config)
	})

	// Serve static files
	// First try relative to current working directory
	staticDir := "./static"
	if _, err := os.Stat(staticDir); os.IsNotExist(err) {
		// Try relative to the source file location
		staticDir = "./cmd/chat/static"
		if _, err := os.Stat(staticDir); os.IsNotExist(err) {
			// Try one more option for when running from project root
			_, filename, _, _ := runtime.Caller(0)
			staticDir = filepath.Join(filepath.Dir(filename), "static")
		}
	}

	fs := http.FileServer(http.Dir(staticDir))
	http.Handle("/", fs)

	// Start server
	addr := fmt.Sprintf(":%s", *port)
	log.Printf("Starting chat server on http://localhost%s", addr)
	log.Printf("WebSocket endpoint: ws://localhost%s/ws", addr)
	log.Printf("Serving static files from: %s", staticDir)

	if err := http.ListenAndServe(addr, nil); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}
