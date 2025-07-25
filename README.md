# 🤖 V3 Agent - Advanced AI Assistant

Intelligent assistant with multi-LLM support, semantic search, iterative function calling, and extensible tool architecture.

## ✨ Key Features

- **🧠 Multi-LLM Support**: OpenAI, Anthropic Claude, Google Gemini with automatic fallback
- **🔧 Advanced Function Calling**: LLM-driven tool selection with recursive execution (up to 20 iterations)
- **🔍 Semantic Knowledge Base**: Intelligent document search using textSearch library with relevance scoring
- **🔄 Iterative Search Workflow**: Automatic keyword refinement and search strategy adaptation
- **💭 Smart Memory**: Contextual conversation memory with intelligent compression
- **⚡ Interactive CLI**: Real-time conversations with thinking indicator and ESC cancellation
- **🔌 Extensible Architecture**: Plugin-based tools, restricted filesystem, and configurable providers

## 🚀 Quick Start

### Prerequisites
- Go 1.21+
- API keys for at least one LLM provider

### Dependencies
- **textSearch**: Semantic search engine library (github.com/scorredoira/textSearch)
- **whatlanggo**: Language detection for multi-language support

### Installation

```bash
git clone <repository-url>
cd agent
go mod tidy
```

### Configuration

```bash
cp config.json.example config.json
# Edit config.json with your API keys
```

### Run CLI

```bash
go run cmd/main.go
```

### Library Usage

```go
package main

import "github.com/santiagocorredoira/agent/agent"

func main() {
    config := agent.AgentConfig{
        ConfigPath: "config.json",
        StorageDir: "./agent_memory",
    }
    
    v3agent, _ := agent.NewV3Agent(config)
    defer v3agent.Shutdown()

    session, _ := v3agent.StartConversation()
    response, _ := v3agent.SendMessage("What tools do you have available?")
    
    fmt.Println(response.Content)
}
```

## 🏗️ Project Structure

```
agent/
├── agent/              # Core agent logic
│   ├── llm/           # LLM providers (OpenAI, Anthropic, Gemini)
│   ├── memory/        # Conversation memory system
│   ├── tools/         # Extensible tool system with kbase integration
│   ├── planner/       # Task planning and tool selection
│   └── config/        # Configuration management
├── cmd/               # Interactive CLI
├── kbase/            # Knowledge base (gitignored - add your docs here)
├── examples/          # Usage examples
└── docs/             # Project documentation
```

## 🔧 Advanced Tool System

- **📚 `kbase` Tool**: Semantic knowledge base search with relevance scoring and content extraction
- **🔍 Search Engine**: Powered by textSearch library for intelligent document discovery
- **📁 File Operations**: Secure file access through RestrictedFS
- **🌐 HTTP Integration**: API calls and web requests
- **🧠 Iterative Search**: LLM automatically refines search strategies up to 20 times

## 💡 Intelligent Function Calling

The agent uses advanced LLM-driven tool selection with iterative refinement:

```
🤖 You: "How do I authenticate with the API?"
⠋ Thinking… (2.3s · esc to interrupt)

# Behind the scenes:
# 1. LLM decides to use kbase tool with "authentication" query
# 2. Semantic search finds relevant docs with scores
# 3. LLM evaluates results, may search again with refined keywords
# 4. Process repeats up to 20 times until answer found
# 5. LLM synthesizes final response from all gathered information

🤖 Agent: According to the documentation, authentication requires...
```

## 📝 Configuration Example

```json
{
  "llm": {
    "default_provider": "openai",
    "fallback_order": ["openai", "anthropic", "mock"],
    "providers": {
      "openai": {
        "api_key": "your-openai-key",
        "model": "gpt-4o",
        "enabled": true
      },
      "anthropic": {
        "api_key": "your-anthropic-key", 
        "model": "claude-3-5-sonnet-20241022",
        "enabled": true
      }
    }
  },
  "knowledge_base": {
    "path": "./kbase",
    "max_search_attempts": 20
  }
}
```

## 🔍 Knowledge Base & Search Architecture

### Semantic Search Engine
The agent uses a sophisticated search system powered by the `textSearch` library:

- **Filename Scoring**: Exact matches (2.0), contains term (1.5), word boundaries (1.0), partial (0.5)
- **Content Scoring**: Multiple occurrences (+0.1 each), all terms bonus (+0.3), partial matches (+0.05)
- **Final Score**: `Filename Score + (Content Score × 0.3)`, capped at 1.0

### Iterative Search Workflow
1. **LLM Analyzes Query**: Determines optimal search keywords
2. **Semantic Search**: Finds relevant documents with relevance scores
3. **Content Extraction**: Extracts relevant snippets from top matches
4. **Result Evaluation**: LLM decides if more searches needed
5. **Keyword Refinement**: Tries different search terms if needed
6. **Iteration**: Repeats up to 20 times until answer found

### Knowledge Base Setup
```bash
# Create knowledge base directory
mkdir kbase

# Add your documentation files (markdown, html, txt, etc.)
cp -r your-docs/* kbase/

# Files are automatically indexed and searchable
# No manual indexing required - textSearch handles it dynamically
```

## 🎯 Development Principles

- **LLM-First Architecture**: Tools selected by LLM reasoning, not hardcoded rules
- **Iterative Intelligence**: Agent persists through multiple search attempts until answer found
- **Semantic Understanding**: Uses textSearch library for intelligent document discovery
- **Secure by Design**: RestrictedFS prevents access outside designated directories
- **Graceful Degradation**: Multiple fallback strategies for reliability
- **Depth-Limited Recursion**: Prevents infinite loops while allowing thorough searches

## 🧪 Testing

```bash
# Build everything
go build ./...

# Test CLI with knowledge base
go run cmd/main.go

# Test with verbose logging
go run cmd/main.go -v

# Test iterative search capabilities
# Try: "How do I authenticate?" to see multiple search attempts
```

## 🐛 Debugging & Interaction Logging

The agent includes powerful debugging capabilities to track and analyze all LLM interactions:

### Enable Logging in CLI

```bash
# Enable interaction logging with default directory (./logs)
./agent --log-interactions

# Custom log directory
./agent --log-interactions --log-dir ./debug_logs

# Verbose mode with logging
./agent --log-interactions -v
```

### Log File Structure

All conversations are saved as text files with process isolation:

```
logs/
├── process_12345/                                    # CLI process 12345
│   ├── session_cli_12345_1753457794315824000.txt    # CLI sessions
│   └── session_cli_12345_1753457826296809000.txt    # More sessions
├── process_12346/                                    # Different process
│   └── session_test_12346_1753457826296809000.txt   # Test sessions
└── process_*/                                       # Process isolation
```

### Viewing Logs

```bash
# List all processes and sessions
ls -la logs/
ls -la logs/process_*/

# View a specific conversation (human-readable text format)
cat logs/process_12345/session_cli_12345_*.txt

# View recent interactions
tail -f logs/process_*/session_*.txt

# Search for specific content across all logs
grep -r "authentication" logs/

# Find sessions with errors
grep -r "ERROR:" logs/
```

### Test Examples

Create tests in `examples/` directory (see `examples/booking_api_test.go`):

```go
// Enable logging for your tests
logger := llm.NewInteractionLogger(&llm.LoggerConfig{
    Enabled: true,
    LogDir:  "./logs/test_sessions",
})

// Sessions are automatically saved with full request/response data
// including function calls, tokens used, and timing information
```

### Mock Scenario Generation

Convert real conversations to reproducible test scenarios:

```go
debugManager := llm.NewDebugManager(logger)
scenario, _ := debugManager.GenerateMockScenario("session_id")
// Use scenario with MockProvider for testing
```

### What Gets Logged

The text format includes:
- **Session Headers**: Start/end times and session IDs
- **User Messages**: All user inputs with timestamps
- **Assistant Responses**: Complete AI responses
- **Tool Calls**: Function names and arguments
- **Errors**: Any failures or issues encountered
- **Timing**: Response duration for each interaction
- **Provider Info**: Which LLM provider was used

## 📚 Documentation

- **[Development Principles](docs/DEVELOPMENT_PRINCIPLES.md)** 

## 🚀 Latest Architecture

- ✅ **Semantic Search Engine**: Integration with textSearch library for intelligent document discovery
- ✅ **Iterative Search Workflow**: LLM automatically refines search strategies (max 20 iterations)
- ✅ **Advanced Function Calling**: Recursive tool execution with depth limiting and fallback strategies
- ✅ **Knowledge Base Tool**: Sophisticated `kbase` tool with relevance scoring and content extraction
- ✅ **Security**: RestrictedFS prevents access outside knowledge base directory
- ✅ **Modern CLI**: Thinking indicator with timer, ESC cancellation, and verbose logging
- ✅ **Configurable Limits**: Customizable search attempts, token limits, and timeout controls

---

Built with modern AI principles for maximum flexibility and accuracy.