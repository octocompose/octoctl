// Package config provides low level config handling for octoctl.
package config

import (
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"dario.cat/mergo"
	"github.com/octocompose/octoctl/pkg/codecs"
)

func isAlphaNumeric(s string) bool {
	for _, v := range s {
		if v < '0' || v > '9' {
			return false
		}
	}

	return true
}

func walkMap(sections []string, in map[string]any) (map[string]any, error) {
	data := in

	for i := 0; i < len(sections); i++ {
		section := sections[i]

		if i+1 < len(sections) && isAlphaNumeric(sections[i+1]) {
			snum, err := strconv.ParseInt(sections[i+1], 10, 64)
			if err != nil {
				return data, fmt.Errorf("while parsing the section number: %w", err)
			}

			sliceData, err := SingleGet(data, section, []any{})
			if err != nil {
				return data, fmt.Errorf("%w: %s", err, strings.Join(sections, "."))
			}

			if int64(len(sliceData)) <= snum {
				return data, fmt.Errorf("%w: %s", ErrNotExistent, strings.Join(sections, "."))
			}

			tmpData, ok := sliceData[snum].(map[string]any)
			if !ok {
				return data, fmt.Errorf("%w: %s", ErrNotExistent, strings.Join(sections, "."))
			}

			data = tmpData
			i++

			continue
		}

		var err error
		if data, err = SingleGet(data, section, map[string]any{}); err != nil {
			return data, fmt.Errorf("%w: %s", err, strings.Join(sections, "."))
		}
	}

	return data, nil
}

// Read reads the config from the given URL.
func Read(myURL *url.URL) (map[string]any, error) {
	switch myURL.Scheme {
	case "file":
		return readFileURL(myURL)
	default:
		return map[string]any{}, ErrUnknownScheme
	}
}

// ParseMap parses the config from config.Read into the given struct.
// Param target should be a pointer to the config to parse into.
func ParseMap[TMap any](sections []string, key string, config map[string]any, target TMap) error {
	data, err := walkMap(sections, config)
	if err != nil {
		return err
	}

	data, err = SingleGet(data, key, map[string]any{})
	if err != nil {
		return err
	}

	codec, err := codecs.GetMime(codecs.MimeJSON)
	if err != nil {
		return err
	}

	b, err := codec.Marshal(data)
	if err != nil {
		return err
	}

	if err := codec.Unmarshal(b, target); err != nil {
		return err
	}

	return nil
}

// ParseSlice parses the config from config.Read into the given slice.
// Param target should be a pointer to the slice to parse into.
func ParseSlice[TSlice any](sections []string, key string, config map[string]any, target TSlice) error {
	var (
		data map[string]any
		err  error
	)

	if len(sections) > 1 {
		data, err = walkMap(sections, config)
		if err != nil {
			return err
		}
	} else {
		data = config
	}

	sliceData, err := SingleGet(data, key, []any{})
	if err != nil {
		return err
	}

	codec, err := codecs.GetMime(codecs.MimeJSON)
	if err != nil {
		return err
	}

	b, err := codec.Marshal(sliceData)
	if err != nil {
		return err
	}

	if err := codec.Unmarshal(b, target); err != nil {
		return err
	}

	return nil
}

// Merge merges the given source into the destination.
func Merge[T any](dst *T, src T) error {
	return mergo.Merge(dst, src, mergo.WithOverride)
}

// Dump is a helper function to dump config to []byte.
func Dump(codecMime string, config map[string]any) ([]byte, error) {
	codec, err := codecs.GetMime(codecMime)
	if err != nil {
		return nil, err
	}

	return codec.Marshal(config)
}

// Write writes the config to the given path.
func Write(path string, config map[string]any) error {
	fp, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600) //nolint:gosec
	if err != nil {
		return err
	}

	defer func() {
		if err := fp.Close(); err != nil {
			slog.Error("failed to close config file", "path", path, "error", err)
		}
	}()

	codec, err := codecs.GetExtension(filepath.Ext(path))
	if err != nil {
		return err
	}

	return codec.NewEncoder(fp).Encode(config)
}
