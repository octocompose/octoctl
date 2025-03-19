package config

import (
	"encoding/base64"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"path/filepath"

	"github.com/octocompose/octoctl/pkg/codecs"
)

func readFileURL(u *url.URL) (map[string]any, error) {
	// Handle base64-encoded content from URL parameter.
	if b64Param := u.Query().Get("base64"); b64Param != "" {
		return readFileFromBase64(u.Path, b64Param)
	}

	// Handle regular file path.
	return readFilePath(u.Host + u.Path)
}

// readFileFromBase64 reads config from a base64-encoded string.
func readFileFromBase64(path, b64Content string) (map[string]any, error) {
	result := map[string]any{}

	// Get codec for file extension.
	codec, err := codecs.GetExtension(filepath.Ext(path))
	if err != nil {
		return result, err
	}

	// Decode base64 content.
	data, err := base64.URLEncoding.DecodeString(b64Content)
	if err != nil {
		return result, fmt.Errorf("failed to decode base64 config: %w", err)
	}

	// Unmarshal the data.
	if err := codec.Unmarshal(data, &result); err != nil {
		return result, fmt.Errorf("failed to unmarshal base64 config: %w", err)
	}

	return result, nil
}

// readFilePath reads config from a filesystem path.
func readFilePath(path string) (map[string]any, error) {
	result := map[string]any{}

	// Get codec for file extension.
	codec, err := codecs.GetExtension(filepath.Ext(path))
	if err != nil {
		return result, err
	}

	// Open and read the file.
	fh, err := os.Open(filepath.Clean(path))
	if err != nil {
		slog.Error("failed to open config file", "path", path, "error", err)
		return result, err
	}

	defer func() {
		if err := fh.Close(); err != nil {
			slog.Error("failed to close config file", "path", path, "error", err)
		}
	}()

	err = codec.NewDecoder(fh).Decode(&result)

	return result, err
}
