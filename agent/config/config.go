package config

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/santiagocorredoira/agent/agent/llm"
)

// Config representa la configuración completa del agente
type Config struct {
	LLM           LLMConfig           `json:"llm"`
	Agent         AgentConfig         `json:"agent"`
	Chat          ChatConfig          `json:"chat"`
	CLI           CLIConfig           `json:"cli"`
	Tools         ToolsConfig         `json:"tools"`
	Search        SearchConfig        `json:"search"`
	Security      SecurityConfig      `json:"security"`
	KnowledgeBase KnowledgeBaseConfig `json:"kbase"`
}

// LLMConfig configuración de proveedores LLM
type LLMConfig struct {
	DefaultProvider string                    `json:"default_provider"`
	FallbackOrder   []string                  `json:"fallback_order"`
	Timeout         time.Duration             `json:"timeout"`
	Providers       map[string]ProviderConfig `json:"providers"`
}

// ProviderConfig configuración de un proveedor específico
type ProviderConfig struct {
	APIKey      string            `json:"api_key"`
	BaseURL     string            `json:"base_url,omitempty"`
	Model       string            `json:"model"`
	MaxTokens   int               `json:"max_tokens"`
	Temperature float64           `json:"temperature"`
	Enabled     bool              `json:"enabled"`
	Extra       map[string]string `json:"extra,omitempty"`
}

// AgentConfig configuración general del agente
type AgentConfig struct {
	Name        string `json:"name"`
	Version     string `json:"version"`
	AutoMode    bool   `json:"auto_mode"`
	Interactive bool   `json:"interactive"`
	LogLevel    string `json:"log_level"`
}

// ChatConfig configuración del chat web
type ChatConfig struct {
	Title string `json:"title"`
}

// CLIConfig configuración del CLI
type CLIConfig struct {
	Prompt       string `json:"prompt"`
	ShowTokens   bool   `json:"show_tokens"`
	ShowTimings  bool   `json:"show_timings"`
	HistorySize  int    `json:"history_size"`
	EnableColors bool   `json:"enable_colors"`
}

// ToolsConfig configuración de herramientas
type ToolsConfig struct {
	EnabledTools []string          `json:"enabled_tools"`
	APIEndpoints map[string]string `json:"api_endpoints"`
	MaxRetries   int               `json:"max_retries"`
}

// SearchConfig configuración del motor de búsqueda
type SearchConfig struct {
	DocumentsPath string `json:"documents_path"`
	IndexPath     string `json:"index_path"`
	MaxResults    int    `json:"max_results"`
}

// SecurityConfig configuración de seguridad
type SecurityConfig struct {
	AllowAPIAccess  bool     `json:"allow_api_access"`
	AllowFileAccess bool     `json:"allow_file_access"`
	RestrictedPaths []string `json:"restricted_paths"`
	RequireConfirm  bool     `json:"require_confirm"`
}

// KnowledgeBaseConfig configuración de la base de conocimiento
type KnowledgeBaseConfig struct {
	Path              string `json:"path"`
	MaxSearchAttempts int    `json:"max_search_attempts"`
}

// LoadConfig carga la configuración desde un archivo JSON
func LoadConfig(filename string) (*Config, error) {
	// Si no se proporciona nombre, buscar en ubicaciones estándar
	if filename == "" {
		filename = findConfigFile()
	}

	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file %s: %w", filename, err)
	}

	var config Config
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse config file %s: %w", filename, err)
	}

	// Aplicar valores por defecto
	config.applyDefaults()

	return &config, nil
}

// LoadConfigOrDefault carga config o devuelve configuración por defecto
func LoadConfigOrDefault(filename string) *Config {
	config, err := LoadConfig(filename)
	if err != nil {
		fmt.Printf("Warning: %v. Using default configuration.\n", err)
		return DefaultConfig()
	}
	return config
}

// SaveConfig guarda la configuración a un archivo JSON
func SaveConfig(config *Config, filename string) error {
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := os.WriteFile(filename, data, 0644); err != nil {
		return fmt.Errorf("failed to write config file %s: %w", filename, err)
	}

	return nil
}

// DefaultConfig devuelve una configuración por defecto
func DefaultConfig() *Config {
	return &Config{
		LLM: LLMConfig{
			DefaultProvider: "mock",
			FallbackOrder:   []string{"anthropic", "openai", "gemini", "mock"},
			Timeout:         30 * time.Second,
			Providers: map[string]ProviderConfig{
				"anthropic": {
					APIKey:      "", // Se llenará desde variables de entorno
					Model:       "claude-3-5-sonnet-20241022",
					MaxTokens:   4096,
					Temperature: 0.7,
					Enabled:     true,
				},
				"openai": {
					APIKey:      "",
					Model:       "gpt-4o",
					MaxTokens:   4096,
					Temperature: 0.7,
					Enabled:     true,
				},
				"gemini": {
					APIKey:      "",
					Model:       "gemini-1.5-pro",
					MaxTokens:   4096,
					Temperature: 0.7,
					Enabled:     true,
				},
				"mock": {
					Model:       "mock-model",
					MaxTokens:   4096,
					Temperature: 0.7,
					Enabled:     true,
				},
			},
		},
		Agent: AgentConfig{
			Name:        "Club Management Agent",
			Version:     "0.1.0",
			AutoMode:    false,
			Interactive: true,
			LogLevel:    "info",
		},
		CLI: CLIConfig{
			Prompt:       "🧑 You: ",
			ShowTokens:   true,
			ShowTimings:  true,
			HistorySize:  100,
			EnableColors: true,
		},
		Tools: ToolsConfig{
			EnabledTools: []string{"api_call", "file_read", "search_docs"},
			APIEndpoints: map[string]string{
				"your_api": "https://api.example.com",
			},
			MaxRetries: 3,
		},
		Search: SearchConfig{
			DocumentsPath: "./docs",
			IndexPath:     "./index",
			MaxResults:    10,
		},
		Security: SecurityConfig{
			AllowAPIAccess:  true,
			AllowFileAccess: true,
			RestrictedPaths: []string{"/etc", "/sys", "/proc"},
			RequireConfirm:  true,
		},
		KnowledgeBase: KnowledgeBaseConfig{
			Path:              "./kbase",
			MaxSearchAttempts: 20,
		},
	}
}

// GetLLMConfig convierte a configuración de LLM
func (c *Config) GetLLMConfig(providerName string) *llm.Config {
	providerConfig, exists := c.LLM.Providers[providerName]
	if !exists {
		return nil
	}

	// Intentar obtener API key desde variables de entorno si no está en config
	apiKey := providerConfig.APIKey
	if apiKey == "" {
		switch providerName {
		case "anthropic":
			apiKey = os.Getenv("ANTHROPIC_API_KEY")
		case "openai":
			apiKey = os.Getenv("OPENAI_API_KEY")
		case "gemini":
			apiKey = os.Getenv("GEMINI_API_KEY")
		}
	}

	return &llm.Config{
		APIKey:      apiKey,
		BaseURL:     providerConfig.BaseURL,
		Model:       providerConfig.Model,
		MaxTokens:   providerConfig.MaxTokens,
		Temperature: providerConfig.Temperature,
		Timeout:     c.LLM.Timeout,
		Extra:       providerConfig.Extra,
	}
}

// applyDefaults aplica valores por defecto a campos faltantes
func (c *Config) applyDefaults() {
	if c.LLM.Timeout == 0 {
		c.LLM.Timeout = 30 * time.Second
	}
	if c.CLI.HistorySize == 0 {
		c.CLI.HistorySize = 100
	}
	if c.Tools.MaxRetries == 0 {
		c.Tools.MaxRetries = 3
	}
	if c.Search.MaxResults == 0 {
		c.Search.MaxResults = 10
	}
	if c.KnowledgeBase.Path == "" {
		c.KnowledgeBase.Path = "./kbase"
	}
	if c.KnowledgeBase.MaxSearchAttempts == 0 {
		c.KnowledgeBase.MaxSearchAttempts = 20
	}
}

// findConfigFile busca el archivo de configuración en ubicaciones estándar
func findConfigFile() string {
	locations := []string{
		"config.json",
		"./config/config.json",
		"./config/default.json",
		os.ExpandEnv("$HOME/.agent/config.json"),
	}

	for _, location := range locations {
		if _, err := os.Stat(location); err == nil {
			return location
		}
	}

	return "config.json" // Fallback
}
