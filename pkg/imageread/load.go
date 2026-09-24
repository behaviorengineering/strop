package imageread

import (
	"context"
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
// Prefer LoadBytesContext when the caller has a cancelable context.
func LoadBytes(pathOrURL string) ([]byte, string, error) {
	return LoadBytesContext(context.Background(), pathOrURL)
}

// LoadBytesContext loads image bytes and honors ctx cancellation for HTTP fetches.
func LoadBytesContext(ctx context.Context, pathOrURL string) ([]byte, string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	pathOrURL = strings.TrimSpace(pathOrURL)
	if pathOrURL == "" {
		return nil, "", invalid("LoadBytes", "image path or URL is empty")
	}
	lower := strings.ToLower(pathOrURL)
	if strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://") {
		return loadHTTP(ctx, pathOrURL)
	}
	return loadFile(pathOrURL)
}

func loadFile(path string) ([]byte, string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, "", wrap("LoadBytes", fmt.Errorf("resolve image path: %w", err))
	}
	info, err := os.Stat(abs)
	if err != nil {
		return nil, "", wrap("LoadBytes", fmt.Errorf("stat image file: %w", err))
	}
	if info.IsDir() {
		return nil, "", invalid("LoadBytes", fmt.Sprintf("image path is a directory: %s", abs))
	}
	if info.Size() > maxImageBytes {
		return nil, "", tooLarge("LoadBytes", fmt.Sprintf("image file exceeds %d bytes", maxImageBytes))
	}
	data, err := os.ReadFile(abs)
	if err != nil {
		return nil, "", wrap("LoadBytes", fmt.Errorf("read image file: %w", err))
	}
	return data, mimeFromPath(abs), nil
}

func loadHTTP(ctx context.Context, url string) (data []byte, mime string, err error) {
	client := &http.Client{Timeout: 60 * time.Second}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, "", wrap("LoadBytes", fmt.Errorf("fetch image URL: %w", err))
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, "", wrap("LoadBytes", fmt.Errorf("fetch image URL: %w", err))
	}
	defer func() {
		if cerr := resp.Body.Close(); cerr != nil && err == nil {
			err = wrap("LoadBytes", fmt.Errorf("close image response: %w", cerr))
		}
	}()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, "", wrap("LoadBytes", fmt.Errorf("fetch image URL: HTTP %d", resp.StatusCode))
	}
	data, err = io.ReadAll(io.LimitReader(resp.Body, maxImageBytes+1))
	if err != nil {
		return nil, "", wrap("LoadBytes", fmt.Errorf("read image response: %w", err))
	}
	if len(data) > maxImageBytes {
		return nil, "", tooLarge("LoadBytes", fmt.Sprintf("image download exceeds %d bytes", maxImageBytes))
	}
	mime = resp.Header.Get("Content-Type")
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
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".gif":
		return "image/gif"
	case ".webp":
		return "image/webp"
	default:
		return "application/octet-stream"
	}
}
