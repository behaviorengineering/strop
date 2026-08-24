package imageread

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const maxImageBytes = 20 << 20 // 20 MiB.

// LoadBytes loads image bytes from a local path or http(s) URL.
func LoadBytes(pathOrURL string) ([]byte, string, error) {
	pathOrURL = strings.TrimSpace(pathOrURL)
	if pathOrURL == "" {
		return nil, "", fmt.Errorf("image path or URL is empty")
	}
	lower := strings.ToLower(pathOrURL)
	if strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://") {
		return loadHTTP(pathOrURL)
	}
	return loadFile(pathOrURL)
}

func loadFile(path string) ([]byte, string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, "", fmt.Errorf("resolve image path: %w", err)
	}
	info, err := os.Stat(abs)
	if err != nil {
		return nil, "", fmt.Errorf("stat image file: %w", err)
	}
	if info.IsDir() {
		return nil, "", fmt.Errorf("image path is a directory: %s", abs)
	}
	if info.Size() > maxImageBytes {
		return nil, "", fmt.Errorf("image file exceeds %d bytes", maxImageBytes)
	}
	data, err := os.ReadFile(abs)
	if err != nil {
		return nil, "", fmt.Errorf("read image file: %w", err)
	}
	return data, mimeFromPath(abs), nil
}

func loadHTTP(url string) ([]byte, string, error) {
	client := &http.Client{Timeout: 60 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return nil, "", fmt.Errorf("fetch image URL: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, "", fmt.Errorf("fetch image URL: HTTP %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxImageBytes+1))
	if err != nil {
		return nil, "", fmt.Errorf("read image response: %w", err)
	}
	if len(data) > maxImageBytes {
		return nil, "", fmt.Errorf("image download exceeds %d bytes", maxImageBytes)
	}
	mime := resp.Header.Get("Content-Type")
	if idx := strings.Index(mime, ";"); idx >= 0 {
		mime = mime[:idx]
	}
	mime = strings.TrimSpace(mime)
	if mime == "" || !strings.HasPrefix(mime, "image/") {
		mime = mimeFromPath(url)
	}
	return data, mime, nil
}

func mimeFromPath(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".png":
		return "image/png"
	case ".webp":
		return "image/webp"
	case ".gif":
		return "image/gif"
	default:
		return "image/png"
	}
}
