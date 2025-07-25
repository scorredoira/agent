package tools

import (
	"bufio"
	"fmt"
	"os"
	"regexp"
	"strings"
)

// DocumentMetadata represents the metadata extracted from document headers
type DocumentMetadata struct {
	Group    string   `json:"group"`
	Title    string   `json:"title"`
	Keywords []string `json:"keywords"`
	Priority int      `json:"priority"`
	FilePath string   `json:"file_path"`
}

// DocumentParser extracts metadata from documentation files
type DocumentParser struct {
	metadataRegex *regexp.Regexp
}

// NewDocumentParser creates a new document parser
func NewDocumentParser() *DocumentParser {
	// Regex to match <apidoc ... /> tags
	metadataRegex := regexp.MustCompile(`<apidoc\s+([^>]+)\s*/>`)
	return &DocumentParser{
		metadataRegex: metadataRegex,
	}
}

// ParseMetadata extracts metadata from a document file
func (p *DocumentParser) ParseMetadata(filePath string) (*DocumentMetadata, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open file %s: %v", filePath, err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	
	// Only scan the first few lines for metadata
	lineCount := 0
	for scanner.Scan() && lineCount < 10 {
		line := strings.TrimSpace(scanner.Text())
		lineCount++
		
		// Look for apidoc tag
		if strings.Contains(line, "<apidoc") {
			metadata, err := p.parseApidocTag(line, filePath)
			if err == nil {
				return metadata, nil
			}
		}
	}

	// No metadata found - return minimal metadata
	return &DocumentMetadata{
		FilePath: filePath,
		Keywords: []string{},
	}, nil
}

// parseApidocTag parses an apidoc metadata tag
func (p *DocumentParser) parseApidocTag(line, filePath string) (*DocumentMetadata, error) {
	matches := p.metadataRegex.FindStringSubmatch(line)
	if len(matches) < 2 {
		return nil, fmt.Errorf("invalid apidoc tag format")
	}

	attributes := matches[1]
	metadata := &DocumentMetadata{
		FilePath: filePath,
		Keywords: []string{},
		Priority: 0,
	}

	// Parse attributes
	err := p.parseAttributes(attributes, metadata)
	if err != nil {
		return nil, err
	}

	return metadata, nil
}

// parseAttributes parses the attributes string from apidoc tag
func (p *DocumentParser) parseAttributes(attributes string, metadata *DocumentMetadata) error {
	// Simple parser for key="value" pairs
	attrRegex := regexp.MustCompile(`(\w+)="([^"]*)"`)
	matches := attrRegex.FindAllStringSubmatch(attributes, -1)

	for _, match := range matches {
		if len(match) < 3 {
			continue
		}
		
		key := match[1]
		value := match[2]

		switch key {
		case "group":
			metadata.Group = value
		case "title":
			metadata.Title = value
		case "keywords":
			// Split keywords by comma and trim spaces
			keywords := strings.Split(value, ",")
			for _, keyword := range keywords {
				trimmed := strings.TrimSpace(keyword)
				if trimmed != "" {
					metadata.Keywords = append(metadata.Keywords, trimmed)
				}
			}
		case "priority":
			// Parse priority as integer
			if _, err := fmt.Sscanf(value, "%d", &metadata.Priority); err != nil {
				// If parsing fails, keep default 0
			}
		}
	}

	return nil
}

// HasKeyword checks if the document has a specific keyword
func (m *DocumentMetadata) HasKeyword(keyword string) bool {
	for _, k := range m.Keywords {
		if strings.EqualFold(k, keyword) {
			return true
		}
	}
	return false
}

// GetDocumentContent reads the full content of a document
func GetDocumentContent(filePath string) (string, error) {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to read file %s: %v", filePath, err)
	}
	return string(content), nil
}