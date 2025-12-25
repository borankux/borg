package storage

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
)

// Storage handles file storage operations
type Storage struct {
	basePath string
}

// NewStorage creates a new storage instance
func NewStorage(basePath string) (*Storage, error) {
	if err := os.MkdirAll(basePath, 0755); err != nil {
		return nil, fmt.Errorf("failed to create storage directory: %w", err)
	}
	
	// Create subdirectories
	dirs := []string{"files", "artifacts", "tmp"}
	for _, dir := range dirs {
		if err := os.MkdirAll(filepath.Join(basePath, dir), 0755); err != nil {
			return nil, fmt.Errorf("failed to create %s directory: %w", dir, err)
		}
	}
	
	return &Storage{basePath: basePath}, nil
}

// SaveFile saves a file to storage and returns its ID and hash
func (s *Storage) SaveFile(reader io.Reader, filename string) (fileID, hash string, size int64, err error) {
	fileID = uuid.New().String()
	
	// Create file path based on date for organization
	now := time.Now()
	datePath := now.Format("2006/01/02")
	dirPath := filepath.Join(s.basePath, "files", datePath)
	
	if err := os.MkdirAll(dirPath, 0755); err != nil {
		return "", "", 0, fmt.Errorf("failed to create directory: %w", err)
	}
	
	filePath := filepath.Join(dirPath, fileID)
	
	// Create file and calculate hash simultaneously
	file, err := os.Create(filePath)
	if err != nil {
		return "", "", 0, fmt.Errorf("failed to create file: %w", err)
	}
	defer file.Close()
	
	hasher := sha256.New()
	multiWriter := io.MultiWriter(file, hasher)
	
	size, err = io.Copy(multiWriter, reader)
	if err != nil {
		os.Remove(filePath)
		return "", "", 0, fmt.Errorf("failed to write file: %w", err)
	}
	
	hash = hex.EncodeToString(hasher.Sum(nil))
	
	return fileID, hash, size, nil
}

// GetFile returns a reader for the file
func (s *Storage) GetFile(fileID string) (io.ReadCloser, error) {
	// Search for file in date-based subdirectories
	baseDir := filepath.Join(s.basePath, "files")
	
	var filePath string
	err := filepath.Walk(baseDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && filepath.Base(path) == fileID {
			filePath = path
			return filepath.SkipAll
		}
		return nil
	})
	
	if err != nil || filePath == "" {
		return nil, fmt.Errorf("file not found: %s", fileID)
	}
	
	return os.Open(filePath)
}

// GetFilePath returns the full path to a file
func (s *Storage) GetFilePath(fileID string) (string, error) {
	baseDir := filepath.Join(s.basePath, "files")
	
	var filePath string
	err := filepath.Walk(baseDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && filepath.Base(path) == fileID {
			filePath = path
			return filepath.SkipAll
		}
		return nil
	})
	
	if err != nil || filePath == "" {
		return "", fmt.Errorf("file not found: %s", fileID)
	}
	
	return filePath, nil
}

// DeleteFile deletes a file from storage
func (s *Storage) DeleteFile(fileID string) error {
	filePath, err := s.GetFilePath(fileID)
	if err != nil {
		return err
	}
	
	return os.Remove(filePath)
}

// SaveArtifact saves an artifact file
func (s *Storage) SaveArtifact(reader io.Reader, filename string) (artifactID, hash string, size int64, err error) {
	artifactID = uuid.New().String()
	
	now := time.Now()
	datePath := now.Format("2006/01/02")
	dirPath := filepath.Join(s.basePath, "artifacts", datePath)
	
	if err := os.MkdirAll(dirPath, 0755); err != nil {
		return "", "", 0, fmt.Errorf("failed to create directory: %w", err)
	}
	
	filePath := filepath.Join(dirPath, artifactID)
	
	file, err := os.Create(filePath)
	if err != nil {
		return "", "", 0, fmt.Errorf("failed to create file: %w", err)
	}
	defer file.Close()
	
	hasher := sha256.New()
	multiWriter := io.MultiWriter(file, hasher)
	
	size, err = io.Copy(multiWriter, reader)
	if err != nil {
		os.Remove(filePath)
		return "", "", 0, fmt.Errorf("failed to write file: %w", err)
	}
	
	hash = hex.EncodeToString(hasher.Sum(nil))
	
	return artifactID, hash, size, nil
}

// GetArtifact returns a reader for an artifact
func (s *Storage) GetArtifact(artifactID string) (io.ReadCloser, error) {
	baseDir := filepath.Join(s.basePath, "artifacts")
	
	var filePath string
	err := filepath.Walk(baseDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && filepath.Base(path) == artifactID {
			filePath = path
			return filepath.SkipAll
		}
		return nil
	})
	
	if err != nil || filePath == "" {
		return nil, fmt.Errorf("artifact not found: %s", artifactID)
	}
	
	return os.Open(filePath)
}

// GetArtifactPath returns the full path to an artifact
func (s *Storage) GetArtifactPath(artifactID string) (string, error) {
	baseDir := filepath.Join(s.basePath, "artifacts")
	
	var filePath string
	err := filepath.Walk(baseDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && filepath.Base(path) == artifactID {
			filePath = path
			return filepath.SkipAll
		}
		return nil
	})
	
	if err != nil || filePath == "" {
		return "", fmt.Errorf("artifact not found: %s", artifactID)
	}
	
	return filePath, nil
}

// DeleteArtifact deletes an artifact from storage
func (s *Storage) DeleteArtifact(artifactID string) error {
	filePath, err := s.GetArtifactPath(artifactID)
	if err != nil {
		return err
	}
	
	return os.Remove(filePath)
}

