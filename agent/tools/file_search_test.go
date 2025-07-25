package tools

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFileSearchTool(t *testing.T) {
	// Create a temporary test directory structure
	testDir := t.TempDir()

	// Create test files
	testFiles := []string{
		"kbase/api_docs/models/customers.html",
		"kbase/api_docs/models/customerGroups.html",
		"kbase/api_docs/models/billing.html",
		"kbase/api_docs/general/Authentication.html",
		"docs/readme.md",
		"docs/api/users.json",
		"config/settings.yaml",
	}

	for _, file := range testFiles {
		fullPath := filepath.Join(testDir, file)
		if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
			t.Fatalf("Failed to create directory: %v", err)
		}
		if err := os.WriteFile(fullPath, []byte("test content"), 0644); err != nil {
			t.Fatalf("Failed to create file %s: %v", file, err)
		}
	}

	// Change to test directory
	originalDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get current directory: %v", err)
	}
	defer os.Chdir(originalDir)

	if err := os.Chdir(testDir); err != nil {
		t.Fatalf("Failed to change to test directory: %v", err)
	}

	tool := NewFileSearchTool()
	ctx := context.Background()

	tests := []struct {
		name          string
		pattern       string
		expectedFiles []string
		expectError   bool
		expectNoFiles bool
	}{
		{
			name:    "Find customer files with **",
			pattern: "**/*customer*",
			expectedFiles: []string{
				"kbase/api_docs/models/customers.html",
				"kbase/api_docs/models/customerGroups.html",
			},
		},
		{
			name:    "Find all HTML files",
			pattern: "**/*.html",
			expectedFiles: []string{
				"kbase/api_docs/models/customers.html",
				"kbase/api_docs/models/customerGroups.html",
				"kbase/api_docs/models/billing.html",
				"kbase/api_docs/general/Authentication.html",
			},
		},
		{
			name:    "Find specific file",
			pattern: "**/customers.html",
			expectedFiles: []string{
				"kbase/api_docs/models/customers.html",
			},
		},
		{
			name:    "Find files in specific directory",
			pattern: "api_docs/**/*.html",
			expectedFiles: []string{
				"kbase/api_docs/models/customers.html",
				"kbase/api_docs/models/customerGroups.html",
				"kbase/api_docs/models/billing.html",
				"kbase/api_docs/general/Authentication.html",
			},
		},
		{
			name:          "Simple pattern without ** (outside kbase)",
			pattern:       "docs/*",
			expectNoFiles: true, // Should return no files as it's outside kbase
		},
		{
			name:          "Non-existent pattern",
			pattern:       "**/*nonexistent*",
			expectNoFiles: true,
		},
		{
			name:          "Find by extension (outside kbase)",
			pattern:       "**/*.json",
			expectNoFiles: true, // Should return no files as json files are outside kbase
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			params := map[string]interface{}{
				"pattern": tt.pattern,
			}

			result, err := tool.Execute(ctx, params)

			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			if !result.Success {
				t.Errorf("Expected success but got failure: %s", result.Error)
				return
			}

			if tt.expectNoFiles {
				if result.Data == nil {
					t.Errorf("Expected data but got nil")
					return
				}
				dataMap, ok := result.Data.(map[string]interface{})
				if !ok {
					t.Errorf("Expected data to be map[string]interface{}")
					return
				}
				matches, ok := dataMap["matches"].([]string)
				if !ok {
					t.Errorf("Expected matches to be []string")
					return
				}
				if len(matches) > 0 {
					t.Errorf("Expected no files but got: %v", matches)
				}
				return
			}

			// Check that we got the expected files
			if result.Data == nil {
				t.Errorf("Expected data but got nil")
				return
			}

			dataMap, ok := result.Data.(map[string]interface{})
			if !ok {
				t.Errorf("Expected data to be map[string]interface{}")
				return
			}

			matches, ok := dataMap["matches"].([]string)
			if !ok {
				t.Errorf("Expected matches to be []string")
				return
			}

			if len(matches) != len(tt.expectedFiles) {
				t.Errorf("Expected %d files, got %d: %v", len(tt.expectedFiles), len(matches), matches)
				return
			}

			// Convert to map for easier checking
			expectedMap := make(map[string]bool)
			for _, file := range tt.expectedFiles {
				expectedMap[file] = true
			}

			for _, match := range matches {
				if !expectedMap[match] {
					t.Errorf("Unexpected file in results: %s", match)
				}
				delete(expectedMap, match)
			}

			if len(expectedMap) > 0 {
				t.Errorf("Missing expected files: %v", expectedMap)
			}
		})
	}
}

func TestFileSearchToolEdgeCases(t *testing.T) {
	tool := NewFileSearchTool()
	ctx := context.Background()

	tests := []struct {
		name        string
		params      map[string]interface{}
		expectError bool
	}{
		{
			name:        "Missing pattern parameter",
			params:      map[string]interface{}{},
			expectError: true,
		},
		{
			name: "Empty pattern",
			params: map[string]interface{}{
				"pattern": "",
			},
			expectError: true,
		},
		{
			name: "Invalid pattern type",
			params: map[string]interface{}{
				"pattern": 123,
			},
			expectError: true,
		},
		{
			name: "Valid pattern with custom path outside kbase",
			params: map[string]interface{}{
				"pattern": "*.go",
				"path":    ".",
			},
			expectError: true, // Should fail due to security restriction
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := tool.Execute(ctx, tt.params)

			if tt.expectError {
				if err == nil && result.Success {
					t.Errorf("Expected error but got success")
				}
				return
			}

			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			if !result.Success {
				t.Errorf("Expected success but got failure: %s", result.Error)
			}
		})
	}
}

func TestFileSearchToolRealFiles(t *testing.T) {
	// Test with actual kbase files if they exist
	tool := NewFileSearchTool()
	ctx := context.Background()

	// First create a kbase directory with a test file
	if err := os.MkdirAll("./kbase/test", 0755); err != nil {
		t.Skip("Cannot create kbase directory")
	}
	defer os.RemoveAll("./kbase/test")

	testFile := "./kbase/test/search_test.txt"
	if err := os.WriteFile(testFile, []byte("test content"), 0644); err != nil {
		t.Skip("Cannot create test file")
	}

	// Test finding files in kbase
	params := map[string]interface{}{
		"pattern": "**/*.txt",
	}

	result, err := tool.Execute(ctx, params)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
		return
	}

	if !result.Success {
		t.Errorf("Expected success but got failure: %s", result.Error)
		return
	}

	// Should find the test file we created
	dataMap, ok := result.Data.(map[string]interface{})
	if !ok {
		t.Errorf("Expected data to be map[string]interface{}")
		return
	}

	matches, ok := dataMap["matches"].([]string)
	if !ok {
		t.Errorf("Expected matches to be []string")
		return
	}

	// Should find the test file we created
	if len(matches) == 0 {
		t.Errorf("Expected to find the test file in kbase")
	}

	// Verify the match is our test file
	foundTestFile := false
	for _, match := range matches {
		if filepath.Base(match) == "search_test.txt" {
			foundTestFile = true
			break
		}
	}

	if !foundTestFile {
		t.Errorf("Did not find the expected test file search_test.txt, got: %v", matches)
	}
}

func TestFileSearchSecurityRestrictions(t *testing.T) {
	tool := NewFileSearchTool()
	ctx := context.Background()

	tests := []struct {
		name        string
		params      map[string]interface{}
		expectError bool
		errorMsg    string
	}{
		{
			name: "Attempt to search outside kbase with absolute path",
			params: map[string]interface{}{
				"pattern": "*.go",
				"path":    "/tmp",
			},
			expectError: true,
			errorMsg:    "Access denied",
		},
		{
			name: "Attempt to search with relative path outside kbase",
			params: map[string]interface{}{
				"pattern": "*.go",
				"path":    "../",
			},
			expectError: true,
			errorMsg:    "Access denied",
		},
		{
			name: "Valid search within kbase",
			params: map[string]interface{}{
				"pattern": "*.html",
				"path":    "./kbase",
			},
			expectError: false,
		},
		{
			name: "Valid search with subdirectory in kbase",
			params: map[string]interface{}{
				"pattern": "*.html",
				"path":    "./kbase/api_docs",
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := tool.Execute(ctx, tt.params)

			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			if tt.expectError {
				if result.Success {
					t.Errorf("Expected failure but got success")
				} else if tt.errorMsg != "" && !contains(result.Error, tt.errorMsg) {
					t.Errorf("Expected error containing '%s' but got: %s", tt.errorMsg, result.Error)
				}
			} else {
				if !result.Success {
					t.Errorf("Expected success but got failure: %s", result.Error)
				}
			}
		})
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && strings.Contains(s, substr))
}
