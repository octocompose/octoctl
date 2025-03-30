package octocache

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-orb/go-orb/config"
)

// downloadFile downloads a file from a URL to a local path.
func downloadFile(ctx context.Context, filepath string, myURL *url.URL) (err error) {
	// Create the file.
	out, err := os.Create(filepath) //nolint:gosec
	if err != nil {
		return err
	}

	defer func() {
		if err := out.Close(); err != nil {
			slog.Error("Error while closing the file", "file", filepath, "error", err)
		}
	}()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, myURL.String(), nil)
	if err != nil {
		return err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}

	defer func() {
		if err := resp.Body.Close(); err != nil {
			slog.Error("Error while closing the body", "url", myURL.String(), "error", err)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		if _, err := io.Copy(io.Discard, resp.Body); err != nil {
			slog.Error("Error while closing the body", "url", myURL.String(), "error", err)
		}

		return fmt.Errorf("bad response status code '%d', status text: %s", resp.StatusCode, resp.Status)
	}

	// Write the file.
	_, err = io.Copy(out, resp.Body)
	if err != nil {
		return err
	}

	return nil
}

// checkSha256Sum verifies that the file matches the SHA256 checksum in the checksum file.
func checkSha256Sum(filePath, checksumPath string) error {
	// Read the checksum file
	checksumBytes, err := os.ReadFile(checksumPath) //nolint:gosec
	if err != nil {
		return fmt.Errorf("failed to read checksum file: %w", err)
	}

	// Parse the checksum (typically in the format "<hash> <filename>")
	checksumStr := strings.TrimSpace(string(checksumBytes))
	// Extract just the hash if it includes a filename
	if parts := strings.Fields(checksumStr); len(parts) > 0 {
		checksumStr = parts[0]
	}

	// Calculate the SHA256 hash of the file
	fileBytes, err := os.ReadFile(filePath) //nolint:gosec
	if err != nil {
		return fmt.Errorf("failed to read file: %w", err)
	}

	fileHash := sha256.Sum256(fileBytes)
	fileHashStr := hex.EncodeToString(fileHash[:])

	// Compare the hashes
	if !strings.EqualFold(fileHashStr, checksumStr) {
		return fmt.Errorf("checksum mismatch: expected %s, got %s", checksumStr, fileHashStr)
	}

	return nil
}

// Path returns the path to the cache directory for a given project and paths.
func Path(projectID string, paths ...string) (string, error) {
	userCacheDir, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}

	dir := filepath.Join(append([]string{userCacheDir, "octocompose", projectID}, paths...)...)
	if err := os.MkdirAll(filepath.Dir(dir), 0o700); err != nil {
		return "", fmt.Errorf("while creating the cache directory '%s': %w", filepath.Dir(dir), err)
	}

	return dir, nil
}

// CachedURL returns a cached version of the given URL.
func CachedURL(
	ctx context.Context,
	projectID string,
	url *config.URL,
	sha256Url *config.URL,
	cacheType string,
	shaFile bool,
) (*config.URL, error) {

	if url.Scheme == "file" {
		return url, nil
	}

	var (
		err        error
		cachedPath string
	)

	if shaFile {
		sha256sum := sha256.Sum256([]byte(url.URL.String()))
		ext := filepath.Ext(url.URL.Path)

		cachedPath, err = Path(projectID, cacheType, hex.EncodeToString(sha256sum[:16])+ext)
		if err != nil {
			return nil, err
		}
	} else {
		cachedPath, err = Path(projectID, cacheType, filepath.Base(url.URL.Path))
		if err != nil {
			return nil, err
		}
	}

	// Check and return if the file already exists.
	if _, err := os.Stat(cachedPath); err == nil {
		return config.NewURL("file://" + cachedPath)
	}

	if sha256Url != nil {
		if err := downloadFile(ctx, cachedPath+".sha256", sha256Url.URL); err != nil {
			return nil, fmt.Errorf("while downloading sha256 sum '%s': %w", sha256Url.URL.String(), err)
		}
	}

	if err := downloadFile(ctx, cachedPath, url.URL); err != nil {
		return nil, fmt.Errorf("while downloading file '%s': %w", url.URL.String(), err)
	}

	// Check sha256 sum.
	if sha256Url != nil {
		if err := checkSha256Sum(cachedPath, cachedPath+".sha256"); err != nil {
			return nil, fmt.Errorf("while checking sha256 sum '%s': %w", cachedPath+".sha256", err)
		}
	}

	return config.NewURL("file://" + cachedPath)
}
