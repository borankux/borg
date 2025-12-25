package downloader

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"borg/solder/internal/client"
)

// Downloader handles file downloads
type Downloader struct {
	client  *client.Client
	workDir string
}

// NewDownloader creates a new downloader
func NewDownloader(c *client.Client, workDir string) *Downloader {
	return &Downloader{
		client:  c,
		workDir: workDir,
	}
}

// DownloadFile downloads a file from mothership
func (d *Downloader) DownloadFile(ctx context.Context, fileID, destPath string) error {
	// Create directory if needed
	dir := filepath.Dir(destPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}
	
	// Create file
	file, err := os.Create(destPath)
	if err != nil {
		return fmt.Errorf("failed to create file: %w", err)
	}
	defer file.Close()
	
	// Download from mothership
	if err := d.client.DownloadFile(ctx, fileID, file); err != nil {
		os.Remove(destPath)
		return fmt.Errorf("failed to download file: %w", err)
	}
	
	return nil
}

// DownloadFiles downloads multiple files
func (d *Downloader) DownloadFiles(ctx context.Context, fileIDs []string, baseDir string) error {
	for i, fileID := range fileIDs {
		destPath := filepath.Join(baseDir, fmt.Sprintf("file_%d", i))
		if err := d.DownloadFile(ctx, fileID, destPath); err != nil {
			return fmt.Errorf("failed to download file %s: %w", fileID, err)
		}
	}
	return nil
}

