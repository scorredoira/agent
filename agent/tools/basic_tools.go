package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// HTTPGetTool performs HTTP GET requests
type HTTPGetTool struct {
	*BaseTool
	client *http.Client
}

// NewHTTPGetTool creates a new HTTP GET tool
func NewHTTPGetTool() *HTTPGetTool {
	tool := &HTTPGetTool{
		BaseTool: NewBaseTool(
			"http_get",
			"Performs HTTP GET requests to retrieve data from web APIs or websites",
			CategoryAPI,
			false, // Low risk operation
			10,    // Low cost
		),
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}

	// Define parameter schema
	schema := &ParameterSchema{
		Type: "object",
		Description: "Parameters for HTTP GET request",
		Properties: map[string]PropertySchema{
			"url": {
				Type:        "string",
				Description: "The URL to send the GET request to",
				Format:      "uri",
			},
			"headers": {
				Type:        "object",
				Description: "Optional HTTP headers to include in the request",
			},
			"timeout": {
				Type:        "number",
				Description: "Request timeout in seconds (default: 30)",
				Default:     30,
				Minimum:     func() *float64 { v := 1.0; return &v }(),
				Maximum:     func() *float64 { v := 300.0; return &v }(),
			},
		},
		Required: []string{"url"},
	}
	tool.SetParameterSchema(schema)

	return tool
}

func (h *HTTPGetTool) Execute(ctx context.Context, params map[string]interface{}) (*ToolResult, error) {
	url, ok := params["url"].(string)
	if !ok {
		return h.CreateErrorResult(
			&ToolError{Type: "parameter_error", Message: "url must be a string", Code: "INVALID_URL"},
			"Invalid URL parameter",
		), nil
	}

	// Create request
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return h.CreateErrorResult(err, "Failed to create HTTP request"), nil
	}

	// Add headers if provided
	if headers, ok := params["headers"].(map[string]interface{}); ok {
		for key, value := range headers {
			if strValue, ok := value.(string); ok {
				req.Header.Set(key, strValue)
			}
		}
	}

	// Set timeout if provided
	if timeoutSecs, ok := params["timeout"].(float64); ok {
		h.client.Timeout = time.Duration(timeoutSecs) * time.Second
	}

	// Execute request
	resp, err := h.client.Do(req)
	if err != nil {
		return h.CreateErrorResult(err, "HTTP request failed"), nil
	}
	defer resp.Body.Close()

	// Read response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return h.CreateErrorResult(err, "Failed to read response body"), nil
	}

	result := map[string]interface{}{
		"status_code": resp.StatusCode,
		"headers":     resp.Header,
		"body":        string(body),
		"url":         url,
	}

	return h.CreateSuccessResult(result, fmt.Sprintf("HTTP GET request to %s completed with status %d", url, resp.StatusCode)), nil
}

func (h *HTTPGetTool) IsAvailable(ctx context.Context) bool {
	return true // HTTP requests are generally always available
}

// FileReadTool reads file contents
type FileReadTool struct {
	*BaseTool
}

// NewFileReadTool creates a new file read tool
func NewFileReadTool() *FileReadTool {
	tool := &FileReadTool{
		BaseTool: NewBaseTool(
			"file_read",
			"Reads the contents of a local file",
			CategoryFile,
			false, // Reading files is generally safe
			5,     // Very low cost
		),
	}

	schema := &ParameterSchema{
		Type: "object",
		Description: "Parameters for reading a file",
		Properties: map[string]PropertySchema{
			"path": {
				Type:        "string",
				Description: "The path to the file to read",
			},
			"encoding": {
				Type:        "string",
				Description: "File encoding (default: utf-8)",
				Default:     "utf-8",
				Enum:        []interface{}{"utf-8", "ascii", "base64"},
			},
			"max_size": {
				Type:        "number",
				Description: "Maximum file size to read in bytes (default: 1MB)",
				Default:     1048576, // 1MB
				Minimum:     func() *float64 { v := 1.0; return &v }(),
				Maximum:     func() *float64 { v := 10485760.0; return &v }(), // 10MB
			},
		},
		Required: []string{"path"},
	}
	tool.SetParameterSchema(schema)

	return tool
}

func (f *FileReadTool) Execute(ctx context.Context, params map[string]interface{}) (*ToolResult, error) {
	path, ok := params["path"].(string)
	if !ok {
		return f.CreateErrorResult(
			&ToolError{Type: "parameter_error", Message: "path must be a string", Code: "INVALID_PATH"},
			"Invalid path parameter",
		), nil
	}

	// Check if file exists
	info, err := os.Stat(path)
	if err != nil {
		return f.CreateErrorResult(err, fmt.Sprintf("File not found: %s", path)), nil
	}

	// Check file size limit
	maxSize := int64(1048576) // Default 1MB
	if maxSizeParam, ok := params["max_size"].(float64); ok {
		maxSize = int64(maxSizeParam)
	}

	if info.Size() > maxSize {
		return f.CreateErrorResult(
			&ToolError{Type: "size_error", Message: "file too large", Code: "FILE_TOO_LARGE"},
			fmt.Sprintf("File size (%d bytes) exceeds maximum allowed size (%d bytes)", info.Size(), maxSize),
		), nil
	}

	// Read file content
	content, err := os.ReadFile(path)
	if err != nil {
		return f.CreateErrorResult(err, fmt.Sprintf("Failed to read file: %s", path)), nil
	}

	contentStr := string(content)
	

	result := map[string]interface{}{
		"path":     path,
		"size":     info.Size(),
		"modified": info.ModTime(),
		"content":  contentStr,
	}

	return f.CreateSuccessResult(result, fmt.Sprintf("Successfully read file: %s (%d bytes)", path, len(content))), nil
}

func (f *FileReadTool) IsAvailable(ctx context.Context) bool {
	return true // File reading is generally always available
}

// FileWriteTool writes content to files
type FileWriteTool struct {
	*BaseTool
}

// NewFileWriteTool creates a new file write tool
func NewFileWriteTool() *FileWriteTool {
	tool := &FileWriteTool{
		BaseTool: NewBaseTool(
			"file_write",
			"Writes content to a local file",
			CategoryFile,
			true, // Writing files can be destructive, requires confirmation
			25,   // Medium cost due to potential impact
		),
	}

	schema := &ParameterSchema{
		Type: "object",
		Description: "Parameters for writing to a file",
		Properties: map[string]PropertySchema{
			"path": {
				Type:        "string",
				Description: "The path where to write the file",
			},
			"content": {
				Type:        "string",
				Description: "The content to write to the file",
			},
			"mode": {
				Type:        "string",
				Description: "Write mode: 'create' (fail if exists), 'overwrite', 'append'",
				Default:     "create",
				Enum:        []interface{}{"create", "overwrite", "append"},
			},
			"create_dirs": {
				Type:        "boolean",
				Description: "Create parent directories if they don't exist",
				Default:     false,
			},
		},
		Required: []string{"path", "content"},
	}
	tool.SetParameterSchema(schema)

	return tool
}

func (f *FileWriteTool) Execute(ctx context.Context, params map[string]interface{}) (*ToolResult, error) {
	path, ok := params["path"].(string)
	if !ok {
		return f.CreateErrorResult(
			&ToolError{Type: "parameter_error", Message: "path must be a string", Code: "INVALID_PATH"},
			"Invalid path parameter",
		), nil
	}

	content, ok := params["content"].(string)
	if !ok {
		return f.CreateErrorResult(
			&ToolError{Type: "parameter_error", Message: "content must be a string", Code: "INVALID_CONTENT"},
			"Invalid content parameter",
		), nil
	}

	mode := "create"
	if modeParam, ok := params["mode"].(string); ok {
		mode = modeParam
	}

	createDirs := false
	if createDirsParam, ok := params["create_dirs"].(bool); ok {
		createDirs = createDirsParam
	}

	// Create parent directories if requested
	if createDirs {
		dir := filepath.Dir(path)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return f.CreateErrorResult(err, fmt.Sprintf("Failed to create directories for: %s", path)), nil
		}
	}

	// Check file existence based on mode
	_, err := os.Stat(path)
	fileExists := err == nil

	switch mode {
	case "create":
		if fileExists {
			return f.CreateErrorResult(
				&ToolError{Type: "file_exists", Message: "file already exists", Code: "FILE_EXISTS"},
				fmt.Sprintf("File already exists and mode is 'create': %s", path),
			), nil
		}
	case "append":
		if !fileExists {
			return f.CreateErrorResult(
				&ToolError{Type: "file_not_found", Message: "file does not exist for append", Code: "FILE_NOT_FOUND"},
				fmt.Sprintf("File does not exist for append mode: %s", path),
			), nil
		}
	}

	// Write file based on mode
	var writeErr error
	switch mode {
	case "append":
		file, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0644)
		if err != nil {
			writeErr = err
		} else {
			_, writeErr = file.WriteString(content)
			file.Close()
		}
	default: // "create" or "overwrite"
		writeErr = os.WriteFile(path, []byte(content), 0644)
	}

	if writeErr != nil {
		return f.CreateErrorResult(writeErr, fmt.Sprintf("Failed to write file: %s", path)), nil
	}

	result := map[string]interface{}{
		"path":         path,
		"bytes_written": len(content),
		"mode":         mode,
	}

	return f.CreateSuccessResult(result, fmt.Sprintf("Successfully wrote file: %s (%d bytes)", path, len(content))), nil
}

func (f *FileWriteTool) IsAvailable(ctx context.Context) bool {
	return true // File writing is generally always available
}

// JSONParseTool parses JSON strings
type JSONParseTool struct {
	*BaseTool
}

// NewJSONParseTool creates a new JSON parse tool
func NewJSONParseTool() *JSONParseTool {
	tool := &JSONParseTool{
		BaseTool: NewBaseTool(
			"json_parse",
			"Parses JSON strings and validates JSON format",
			CategoryData,
			false, // JSON parsing is safe
			5,     // Very low cost
		),
	}

	schema := &ParameterSchema{
		Type: "object",
		Description: "Parameters for JSON parsing",
		Properties: map[string]PropertySchema{
			"json_string": {
				Type:        "string",
				Description: "The JSON string to parse",
			},
			"validate_only": {
				Type:        "boolean",
				Description: "Only validate JSON format without returning parsed data",
				Default:     false,
			},
		},
		Required: []string{"json_string"},
	}
	tool.SetParameterSchema(schema)

	return tool
}

func (j *JSONParseTool) Execute(ctx context.Context, params map[string]interface{}) (*ToolResult, error) {
	jsonString, ok := params["json_string"].(string)
	if !ok {
		return j.CreateErrorResult(
			&ToolError{Type: "parameter_error", Message: "json_string must be a string", Code: "INVALID_JSON_STRING"},
			"Invalid json_string parameter",
		), nil
	}

	validateOnly := false
	if validateOnlyParam, ok := params["validate_only"].(bool); ok {
		validateOnly = validateOnlyParam
	}

	// Parse JSON
	var parsed interface{}
	err := json.Unmarshal([]byte(jsonString), &parsed)
	if err != nil {
		return j.CreateErrorResult(err, "Invalid JSON format"), nil
	}

	result := map[string]interface{}{
		"valid": true,
		"size":  len(jsonString),
	}

	if !validateOnly {
		result["parsed_data"] = parsed
		result["type"] = fmt.Sprintf("%T", parsed)
	}

	message := "JSON is valid"
	if !validateOnly {
		message = "JSON parsed successfully"
	}

	return j.CreateSuccessResult(result, message), nil
}

func (j *JSONParseTool) IsAvailable(ctx context.Context) bool {
	return true // JSON parsing is always available
}

// TextSearchTool searches for text patterns in strings
type TextSearchTool struct {
	*BaseTool
}

// NewTextSearchTool creates a new text search tool
func NewTextSearchTool() *TextSearchTool {
	tool := &TextSearchTool{
		BaseTool: NewBaseTool(
			"text_search",
			"Searches for text patterns within strings using various matching modes",
			CategoryText,
			false, // Text search is safe
			5,     // Very low cost
		),
	}

	schema := &ParameterSchema{
		Type: "object",
		Description: "Parameters for text searching",
		Properties: map[string]PropertySchema{
			"text": {
				Type:        "string",
				Description: "The text to search within",
			},
			"pattern": {
				Type:        "string",
				Description: "The pattern to search for",
			},
			"mode": {
				Type:        "string",
				Description: "Search mode: 'contains', 'starts_with', 'ends_with', 'exact'",
				Default:     "contains",
				Enum:        []interface{}{"contains", "starts_with", "ends_with", "exact"},
			},
			"case_sensitive": {
				Type:        "boolean",
				Description: "Whether the search should be case sensitive",
				Default:     false,
			},
		},
		Required: []string{"text", "pattern"},
	}
	tool.SetParameterSchema(schema)

	return tool
}

func (t *TextSearchTool) Execute(ctx context.Context, params map[string]interface{}) (*ToolResult, error) {
	text, ok := params["text"].(string)
	if !ok {
		return t.CreateErrorResult(
			&ToolError{Type: "parameter_error", Message: "text must be a string", Code: "INVALID_TEXT"},
			"Invalid text parameter",
		), nil
	}

	pattern, ok := params["pattern"].(string)
	if !ok {
		return t.CreateErrorResult(
			&ToolError{Type: "parameter_error", Message: "pattern must be a string", Code: "INVALID_PATTERN"},
			"Invalid pattern parameter",
		), nil
	}

	mode := "contains"
	if modeParam, ok := params["mode"].(string); ok {
		mode = modeParam
	}

	caseSensitive := false
	if caseSensitiveParam, ok := params["case_sensitive"].(bool); ok {
		caseSensitive = caseSensitiveParam
	}

	// Prepare strings for comparison
	searchText := text
	searchPattern := pattern
	if !caseSensitive {
		searchText = strings.ToLower(text)
		searchPattern = strings.ToLower(pattern)
	}

	// Perform search based on mode
	var found bool
	var positions []int

	switch mode {
	case "contains":
		found = strings.Contains(searchText, searchPattern)
		if found {
			// Find all occurrences
			start := 0
			for {
				pos := strings.Index(searchText[start:], searchPattern)
				if pos == -1 {
					break
				}
				positions = append(positions, start+pos)
				start += pos + 1
			}
		}
	case "starts_with":
		found = strings.HasPrefix(searchText, searchPattern)
		if found {
			positions = []int{0}
		}
	case "ends_with":
		found = strings.HasSuffix(searchText, searchPattern)
		if found {
			positions = []int{len(text) - len(pattern)}
		}
	case "exact":
		found = searchText == searchPattern
		if found {
			positions = []int{0}
		}
	}

	result := map[string]interface{}{
		"found":          found,
		"match_count":    len(positions),
		"positions":      positions,
		"mode":           mode,
		"case_sensitive": caseSensitive,
		"text_length":    len(text),
		"pattern_length": len(pattern),
	}

	message := fmt.Sprintf("Search completed: found %d matches", len(positions))

	return t.CreateSuccessResult(result, message), nil
}

func (t *TextSearchTool) IsAvailable(ctx context.Context) bool {
	return true // Text search is always available
}