package tools

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// RestrictedFS implements fs.FS to restrict access to a specific directory
type RestrictedFS struct {
	root string
}

// NewRestrictedFS creates a new restricted filesystem rooted at the given path
func NewRestrictedFS(root string) (*RestrictedFS, error) {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("failed to get absolute path: %w", err)
	}
	
	// Ensure the root exists
	info, err := os.Stat(absRoot)
	if err != nil {
		return nil, fmt.Errorf("root path does not exist: %w", err)
	}
	
	if !info.IsDir() {
		return nil, fmt.Errorf("root path is not a directory")
	}
	
	return &RestrictedFS{root: absRoot}, nil
}

// Open implements fs.FS
func (rfs *RestrictedFS) Open(name string) (fs.File, error) {
	// Validate the path
	if !fs.ValidPath(name) {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrInvalid}
	}
	
	// Construct full path
	fullPath := filepath.Join(rfs.root, filepath.FromSlash(name))
	
	// Ensure the path is within our root
	cleanPath, err := filepath.Abs(fullPath)
	if err != nil {
		return nil, &fs.PathError{Op: "open", Path: name, Err: err}
	}
	
	if !strings.HasPrefix(cleanPath, rfs.root) {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrPermission}
	}
	
	// Open the file
	return os.Open(cleanPath)
}

// ReadDir implements fs.ReadDirFS
func (rfs *RestrictedFS) ReadDir(name string) ([]fs.DirEntry, error) {
	// Validate the path
	if name != "." && !fs.ValidPath(name) {
		return nil, &fs.PathError{Op: "readdir", Path: name, Err: fs.ErrInvalid}
	}
	
	// Construct full path
	fullPath := rfs.root
	if name != "." {
		fullPath = filepath.Join(rfs.root, filepath.FromSlash(name))
	}
	
	// Ensure the path is within our root
	cleanPath, err := filepath.Abs(fullPath)
	if err != nil {
		return nil, &fs.PathError{Op: "readdir", Path: name, Err: err}
	}
	
	if !strings.HasPrefix(cleanPath, rfs.root) {
		return nil, &fs.PathError{Op: "readdir", Path: name, Err: fs.ErrPermission}
	}
	
	// Read the directory
	return os.ReadDir(cleanPath)
}

// Stat implements fs.StatFS
func (rfs *RestrictedFS) Stat(name string) (fs.FileInfo, error) {
	// Validate the path
	if name != "." && !fs.ValidPath(name) {
		return nil, &fs.PathError{Op: "stat", Path: name, Err: fs.ErrInvalid}
	}
	
	// Construct full path
	fullPath := rfs.root
	if name != "." {
		fullPath = filepath.Join(rfs.root, filepath.FromSlash(name))
	}
	
	// Ensure the path is within our root
	cleanPath, err := filepath.Abs(fullPath)
	if err != nil {
		return nil, &fs.PathError{Op: "stat", Path: name, Err: err}
	}
	
	if !strings.HasPrefix(cleanPath, rfs.root) {
		return nil, &fs.PathError{Op: "stat", Path: name, Err: fs.ErrPermission}
	}
	
	// Stat the file
	return os.Stat(cleanPath)
}

// GetRoot returns the root path of the restricted filesystem
func (rfs *RestrictedFS) GetRoot() string {
	return rfs.root
}

// Glob implements glob pattern matching within the restricted filesystem
func (rfs *RestrictedFS) Glob(pattern string) ([]string, error) {
	// Use fs.Glob with our restricted filesystem
	matches, err := fs.Glob(rfs, pattern)
	if err != nil {
		return nil, err
	}
	
	// Convert matches to full paths relative to root
	var result []string
	for _, match := range matches {
		result = append(result, match)
	}
	
	return result, nil
}

// Walk walks the file tree rooted at root within the restricted filesystem
func (rfs *RestrictedFS) Walk(root string, fn fs.WalkDirFunc) error {
	return fs.WalkDir(rfs, root, fn)
}