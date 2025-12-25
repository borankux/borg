package uploader

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"borg/solder/internal/client"
)

// Uploader handles file uploads
type Uploader struct {
	client *client.Client
}

// NewUploader creates a new uploader
func NewUploader(c *client.Client) *Uploader {
	return &Uploader{
		client: c,
	}
}

// UploadArtifact uploads an artifact file to mothership
func (u *Uploader) UploadArtifact(ctx context.Context, taskID, filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()
	
	filename := filepath.Base(filePath)
	resp, err := u.client.UploadArtifact(ctx, taskID, filename, file)
	if err != nil {
		return "", fmt.Errorf("failed to upload artifact: %w", err)
	}
	
	if !resp.Success {
		return "", fmt.Errorf("upload failed: %s", resp.Message)
	}
	
	return resp.ArtifactID, nil
}

// UploadArtifacts uploads multiple artifacts from a directory
func (u *Uploader) UploadArtifacts(ctx context.Context, taskID, dirPath string) ([]string, error) {
	var artifactIDs []string
	
	err := filepath.Walk(dirPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		
		if info.IsDir() {
			return nil
		}
		
		artifactID, err := u.UploadArtifact(ctx, taskID, path)
		if err != nil {
			return fmt.Errorf("failed to upload artifact %s: %w", path, err)
		}
		
		artifactIDs = append(artifactIDs, artifactID)
		return nil
	})
	
	return artifactIDs, err
}

