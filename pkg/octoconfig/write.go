package octoconfig

import (
	"os"
	"path/filepath"

	"github.com/go-orb/go-orb/codecs"
	"github.com/go-orb/go-orb/log"
)

// Write writes the config to the given path.
func Write(path string, config map[string]any) error {
	fp, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600) //nolint:gosec
	if err != nil {
		return err
	}

	defer func() {
		if err := fp.Close(); err != nil {
			log.Error("failed to close config file", "path", path, "error", err)
		}
	}()

	codec, err := codecs.GetExt(filepath.Ext(path))
	if err != nil {
		return err
	}

	return codec.NewEncoder(fp).Encode(config)
}
