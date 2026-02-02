package install

import (
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/schollz/progressbar/v3"
)

// isURL checks if the given path is a URL
func isURL(path string) bool {
	return strings.HasPrefix(path, "http://") ||
		strings.HasPrefix(path, "https://")
}

// getFilenameFromURL extracts filename from URL
func getFilenameFromURL(source string) string {
	u, err := url.Parse(source)
	if err != nil {
		return ""
	}
	parts := strings.Split(u.Path, "/")
	if len(parts) == 0 {
		return ""
	}
	return parts[len(parts)-1]
}

// AssetManager handles asset resolution and cleanup
type AssetManager struct {
	tempDir         string
	downloadedFiles []string
}

// NewAssetManager creates a new asset manager with a temp directory
func NewAssetManager() (*AssetManager, error) {
	tempDir, err := os.MkdirTemp("", "k3air-assets-*")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp directory: %w", err)
	}
	return &AssetManager{
		tempDir:         tempDir,
		downloadedFiles: make([]string, 0),
	}, nil
}

// ResolveAsset returns the local path to use for an asset
// - If source is a local file path that exists, return it as-is
// - If source is a URL, download to temp dir and return temp path
// - If source is a local path that doesn't exist, return error with helpful hint
func (am *AssetManager) ResolveAsset(source, description string) (string, error) {
	if isURL(source) {
		slog.Info("downloading asset", "description", description, "url", source)
		localPath, err := am.download(source)
		if err != nil {
			return "", fmt.Errorf("failed to download %s: %w", description, err)
		}
		am.downloadedFiles = append(am.downloadedFiles, localPath)
		slog.Info("download complete", "path", localPath)
		return localPath, nil
	}

	// Local path - check if file exists
	if _, err := os.Stat(source); err != nil {
		if os.IsNotExist(err) {
			// Provide helpful error message based on file type
			var hint string
			if source == "k3s" || source == "./k3s" {
				hint = `

Please download k3s binary:
  wget https://github.com/k3s-io/k3s/releases/download/v1.28.5+k3s1/k3s
  chmod +x k3s
Or configure a URL in your init.yaml under assets.k3s-binary`
			} else if source == "k3s-airgap-images-amd64.tar.gz" || source == "./k3s-airgap-images-amd64.tar.gz" {
				hint = `

Please download k3s airgap images:
  wget https://github.com/k3s-io/k3s/releases/download/v1.28.5+k3s1/k3s-airgap-images-amd64.tar.gz
Or configure a URL in your init.yaml under assets.k3s-airgap-tarball`
			}
			return "", fmt.Errorf("%s file not found: %s%s", description, source, hint)
		}
		return "", fmt.Errorf("failed to access %s: %w", description, err)
	}
	return source, nil
}

// download downloads a URL to the temp directory with progress bar
func (am *AssetManager) download(urlStr string) (string, error) {
	filename := getFilenameFromURL(urlStr)
	if filename == "" {
		return "", fmt.Errorf("cannot determine filename from URL: %s", urlStr)
	}

	localPath := filepath.Join(am.tempDir, filename)

	// Create file
	outFile, err := os.Create(localPath)
	if err != nil {
		return "", fmt.Errorf("failed to create temp file: %w", err)
	}
	defer outFile.Close()

	// HTTP GET with timeout
	client := &http.Client{
		Timeout: 30 * time.Minute,
	}
	resp, err := client.Get(urlStr)
	if err != nil {
		return "", fmt.Errorf("download request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download failed with status: %s", resp.Status)
	}

	// Progress bar for download
	size := resp.ContentLength
	var writer io.Writer = outFile

	if size > 0 {
		bar := progressbar.NewOptions(int(size),
			progressbar.OptionShowBytes(true),
			progressbar.OptionSetDescription("downloading "+filename))
		writer = io.MultiWriter(outFile, bar)
	}

	// Copy with progress
	_, err = io.Copy(writer, resp.Body)
	if _, ok := writer.(interface{ Flush() }); ok {
		writer.(interface{ Flush() }).Flush()
	}
	fmt.Println() // Newline after progress bar

	if err != nil {
		return "", fmt.Errorf("download failed: %w", err)
	}

	return localPath, nil
}

// Cleanup removes all downloaded files and the temp directory
func (am *AssetManager) Cleanup() error {
	if am.tempDir == "" {
		return nil
	}
	slog.Debug("cleaning up downloaded assets", "temp_dir", am.tempDir)
	err := os.RemoveAll(am.tempDir)
	if err != nil {
		return fmt.Errorf("cleanup failed: %w", err)
	}
	return nil
}
