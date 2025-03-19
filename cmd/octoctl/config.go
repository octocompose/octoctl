package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"iter"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"text/template"

	"github.com/hashicorp/go-multierror"
	"github.com/lithammer/shortuuid/v3"
	"github.com/octocompose/octoctl/pkg/codecs"
	"github.com/octocompose/octoctl/pkg/config"
)

func downloadFile(ctx context.Context, filepath string, myURL *url.URL) (err error) {
	// Create the file
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

	// Write the file
	_, err = io.Copy(out, resp.Body)
	if err != nil {
		return err
	}

	return nil
}

func absURL(dst *url.URL, src *url.URL) bool {
	if !filepath.IsAbs(dst.Path) {
		dst.Scheme = src.Scheme
		dst.Host = src.Host
		dir := filepath.Dir(src.Path)
		dst.Path = filepath.Join(dir, dst.Path)

		return true
	}

	return false
}

func cachedURL(ctx context.Context, projectID string, url *jsonURL) (*jsonURL, error) {
	if url.Scheme == "file" {
		return url, nil
	}

	userCacheDir, err := os.UserCacheDir()
	if err != nil {
		return nil, err
	}

	sha256sum := sha256.Sum256([]byte(url.URL.String()))
	ext := filepath.Ext(url.URL.Path)

	cachedPath := filepath.Join(userCacheDir, "octoctl", projectID, "configs", string(sha256sum[:])+ext)
	if err := os.MkdirAll(filepath.Dir(cachedPath), 0700); err != nil {
		return nil, fmt.Errorf("while creating the cache directory '%s': %w", filepath.Dir(cachedPath), err)
	}

	// Check and return if the file already exists
	if _, err := os.Stat(cachedPath); err == nil {
		return newJSONURL(cachedPath)
	}

	if err := downloadFile(ctx, cachedPath, url.URL); err != nil {
		return nil, fmt.Errorf("while downloading config '%s': %w", url.URL.String(), err)
	}

	return newJSONURL(cachedPath)
}

type jsonURL struct {
	*url.URL
}

func newJSONURL(s string) (*jsonURL, error) {
	u, err := url.Parse(s)
	if err != nil {
		return nil, err
	}

	return &jsonURL{URL: u}, nil
}

func (j *jsonURL) UnmarshalJSON(data []byte) error {
	u, err := url.Parse(strings.Trim(string(data), `"`))
	if err != nil {
		return err
	}

	j.URL = u

	return nil
}

func (j *jsonURL) MarshalJSON() ([]byte, error) {
	if j == nil || j.URL == nil {
		return nil, nil
	}

	return []byte(`"` + j.URL.String() + `"`), nil
}

type configIncludeVersions struct {
	Format string   `json:"format"`
	URL    *jsonURL `json:"url"`
}

// urlConfig represents a configuration include.
type urlConfig struct {
	BaseConfig *Config               `json:"-"`
	URL        *jsonURL              `json:"url"`
	GPG        *jsonURL              `json:"gpg"`
	Cached     *jsonURL              `json:"-"`
	Versions   configIncludeVersions `json:"versions"`
	Data       map[string]any        `json:"data"`
	Includes   []*urlConfig          `json:"-"`
	Repos      []*urlConfig          `json:"-"`
}

func newURLConfig(baseConfig *Config, url *jsonURL) *urlConfig {
	return &urlConfig{BaseConfig: baseConfig, URL: url}
}

func (u *urlConfig) Read(ctx context.Context) error {
	slog.Debug("Reading file", "url", u.URL.String())

	var err error

	u.Cached, err = cachedURL(ctx, u.BaseConfig.ProjectID, u.URL)
	if err != nil {
		return err
	}

	if u.Data == nil {
		data, err := config.Read(u.Cached.URL)
		if err != nil {
			return fmt.Errorf("while reading config '%s': %w", u.Cached.String(), err)
		}

		u.Data = data
	}

	// Parse per config includes
	err = config.ParseSlice([]string{}, "include", u.Data, &u.Includes)
	if err != nil {
		if !errors.Is(err, config.ErrNotExistent) {
			return err
		}
	}

	mErr := &multierror.Error{}

	if err == nil {
		delete(u.Data, "include")

		//nolint:dupl
		for _, include := range u.Includes {
			absURL(include.URL.URL, u.URL.URL)

			if _, ok := u.BaseConfig.KnownURLs[include.URL.String()]; ok {
				slog.Warn("while reading config '%s': duplicate include '%s'", u.Cached.String(), include.URL.String())
				continue
			}

			include.BaseConfig = u.BaseConfig
			if err := include.Read(ctx); err != nil {
				mErr = multierror.Append(mErr, err)
			}

			u.BaseConfig.KnownURLs[include.URL.String()] = struct{}{}
		}
	}

	err = config.ParseSlice([]string{"repos"}, "include", u.Data, &u.Repos)
	if err != nil {
		if !errors.Is(err, config.ErrNotExistent) {
			return err
		}
	}

	if err == nil {
		delete(u.Data["repos"].(map[string]any), "include") //nolint:errcheck

		//nolint:dupl
		for _, repo := range u.Repos {
			absURL(repo.URL.URL, u.URL.URL)

			if _, ok := u.BaseConfig.KnownURLs[repo.URL.String()]; ok {
				slog.Warn("while reading config '%s': duplicate repo '%s'", u.Cached.String(), repo.URL.String())
				continue
			}

			repo.BaseConfig = u.BaseConfig
			if err := repo.Read(ctx); err != nil {
				mErr = multierror.Append(mErr, err)
			}

			u.BaseConfig.KnownURLs[repo.URL.String()] = struct{}{}
		}
	}

	return mErr.ErrorOrNil()
}

// Flatten returns a sequence iterator that yields the urlConfig and all its includes.
func (u *urlConfig) Flatten() iter.Seq[*urlConfig] {
	return iter.Seq[*urlConfig](func(yield func(*urlConfig) bool) {
		// First yield the current command
		if !yield(u) {
			return
		}

		slices.Reverse(u.Includes)

		// Then recursively yield all subcommands
		for _, subCmd := range u.Includes {
			for subCmd2 := range subCmd.Flatten() {
				// Pass each subcommand to the yield function
				// If yield returns false, we stop iteration
				if !yield(subCmd2) {
					return
				}
			}
		}
	})
}

// Config represents a configuration.
type Config struct {
	Paths []*urlConfig

	KnownURLs map[string]struct{}

	// Merged data
	Data map[string]any

	ProjectID string
}

// NewConfig creates a new configuration from the given paths.
func NewConfig(paths []string) (*Config, error) {
	cfg := &Config{Paths: []*urlConfig{}, KnownURLs: map[string]struct{}{}}

	for _, path := range paths {
		myURL, err := newJSONURL(path)
		if err != nil {
			return nil, err
		}

		if myURL.Scheme == "" {
			myURL.Scheme = "file"

			symPath, err := filepath.EvalSymlinks(myURL.Path)
			if err != nil {
				return nil, err
			}

			myURL.Path, err = filepath.Abs(symPath)

			if err != nil {
				return nil, err
			}
		}

		if _, ok := cfg.KnownURLs[myURL.String()]; ok {
			slog.Warn("duplicate URL", "url", myURL.String())
			continue
		}

		cfg.Paths = append(cfg.Paths, newURLConfig(cfg, myURL))
		cfg.KnownURLs[myURL.String()] = struct{}{}
	}

	return cfg, nil
}

// Run runs the configuration.
func (c *Config) Run(ctx context.Context) error {
	if err := c.EnsureProjectID(ctx); err != nil {
		return err
	}

	if err := c.Read(ctx); err != nil {
		return err
	}

	if err := c.Merge(ctx); err != nil {
		return err
	}

	if err := c.ApplyGlobals(); err != nil {
		return err
	}

	if err := c.ApplyServiceTemplates(); err != nil {
		return err
	}

	return nil
}

// EnsureProjectID ensures that the projectID is set in the configuration.
func (c *Config) EnsureProjectID(_ context.Context) error {
	for _, cfg := range c.Paths {
		data, err := config.Read(cfg.URL.URL)
		if err != nil {
			return fmt.Errorf("while reading config '%s': %w", cfg.URL.String(), err)
		}

		tmp, err := config.SingleGet(data, "projectID", "")
		if err == nil {
			c.ProjectID = tmp
			return nil
		}

		cfg.Data = data
	}

	// No projectID found, write a new one to the last given config file.
	cfg := c.Paths[len(c.Paths)-1]
	cfg.Data["projectID"] = shortuuid.New()

	if err := config.Write(cfg.URL.Path, cfg.Data); err != nil {
		return fmt.Errorf("while writing config '%s': %w", cfg.URL.String(), err)
	}

	return nil
}

// Read reads the configuration data from all paths.
func (c *Config) Read(ctx context.Context) error {
	mErr := &multierror.Error{}

	for _, path := range c.Paths {
		if err := path.Read(ctx); err != nil {
			mErr = multierror.Append(mErr, err)
		}
	}

	return mErr.ErrorOrNil()
}

// Merge merges the configuration data from all paths.
func (c *Config) Merge(ctx context.Context) error {
	// Flatten/gather all paths
	paths := []*urlConfig{}

	// Last in first out - reverse CLI config paths
	slices.Reverse(c.Paths)

	for _, path := range c.Paths {
		paths = append(paths, slices.Collect(path.Flatten())...)
	}

	// Reverse the order of paths to merge them in the correct order
	slices.Reverse(paths)

	var mErr *multierror.Error

	for _, path := range paths {
		slog.Debug("Merging config", "path", path.URL.String())

		if err := config.Merge(&c.Data, path.Data); err != nil {
			mErr = multierror.Append(mErr, err)
		}

		data := map[string]any{}

		if myData, ok := c.Data["repos"].(map[string]any); ok {
			data = myData
		}

		for _, repo := range path.Repos {
			for r := range repo.Flatten() {
				// Fix repo URLs
				files := map[string]any{}
				err := config.ParseMap([]string{}, "files", r.Data, &files)

				if err != nil {
					if !errors.Is(err, config.ErrNotExistent) {
						mErr = multierror.Append(mErr, err)
					}
				}

				if err == nil {
					for k, v := range files {
						if u, ok := v.(map[string]any)["url"]; ok { //nolint:errcheck
							url, uErr := url.Parse(u.(string)) //nolint:errcheck
							if uErr != nil {
								mErr = multierror.Append(mErr, uErr)
							}

							absURL(url, r.URL.URL)
							cached, err := cachedURL(ctx, c.ProjectID, &jsonURL{URL: url})
							if err != nil {
								mErr = multierror.Append(mErr, err)
							} else {
								files[k] = map[string]any{"path": cached.Path}
							}
						}
					}

					r.Data["files"] = files
				}

				if err := config.Merge(&data, r.Data); err != nil {
					mErr = multierror.Append(mErr, err)
				}
			}
		}

		c.Data["repos"] = data
	}

	return mErr.ErrorOrNil()
}

type svcConfig struct {
	Globals    string `json:"globals"`
	NoTemplate bool   `json:"noTemplate"`
}

// ApplyGlobals applies the global configuration to each service.
func (c *Config) ApplyGlobals() error {
	var services map[string]any

	if err := config.ParseMap([]string{}, "services", c.Data, &services); err != nil {
		if errors.Is(err, config.ErrNotExistent) {
			return nil
		}

		return fmt.Errorf("while parsing services: %w", err)
	}

	var globals map[string]any

	if err := config.ParseMap([]string{}, "globals", c.Data, &globals); err != nil {
		if errors.Is(err, config.ErrNotExistent) {
			return nil
		}

		return fmt.Errorf("while parsing globals: %w", err)
	}

	for name, _ := range services {
		servicesSvcConfig := svcConfig{}

		if err := config.ParseMap([]string{"services", name, "octocompose"}, "config", c.Data, &servicesSvcConfig); err != nil {
			continue
		}

		if servicesSvcConfig.Globals == "" {
			continue
		}

		myGlobals, ok := globals[servicesSvcConfig.Globals]
		if !ok {
			slog.Error("while applying globals", "globals", servicesSvcConfig.Globals, "service", name)
			return fmt.Errorf("while applying globals to service %s: %s is not defined", name, servicesSvcConfig.Globals)
		}

		myConfig := map[string]any{}
		if err := config.ParseMap([]string{"configs"}, name, c.Data, &myConfig); err != nil {
			if !errors.Is(err, config.ErrNotExistent) {
				slog.Error("while applying globals", "globals", servicesSvcConfig.Globals, "service", name)
				return fmt.Errorf("while applying globals to service %s: %s is not defined", name, servicesSvcConfig.Globals)
			}
		}

		newConfig := map[string]any{}
		if err := config.Merge(&newConfig, myGlobals.(map[string]any)); err != nil { //nolint:errcheck
			slog.Error("while applying globals", "globals", servicesSvcConfig.Globals, "service", name)
			return fmt.Errorf("while applying globals to service %s: %w", name, err)
		}

		if err := config.Merge(&newConfig, myConfig); err != nil {
			slog.Error("while applying globals", "globals", servicesSvcConfig.Globals, "service", name)
			return fmt.Errorf("while applying globals to service %s: %w", name, err)
		}

		if _, ok := c.Data["configs"]; !ok {
			c.Data["configs"] = map[string]any{}
		}

		c.Data["configs"].(map[string]any)[name] = newConfig //nolint:errcheck
	}

	delete(c.Data, "globals")

	return nil
}

// ApplyServiceTemplates applies the service templates.
func (c *Config) ApplyServiceTemplates() error {
	var services map[string]any

	if err := config.ParseMap([]string{}, "services", c.Data, &services); err != nil {
		if errors.Is(err, config.ErrNotExistent) {
			return nil
		}

		return fmt.Errorf("while parsing services: %w", err)
	}

	jsonCodec, err := codecs.GetMime(codecs.MimeJSON)
	if err != nil {
		return fmt.Errorf("while getting JSON codec: %w", err)
	}

	for name, svc := range services {
		jsonB, err := jsonCodec.Marshal(svc)
		if err != nil {
			return fmt.Errorf("while marshaling service %s: %w", name, err)
		}

		t, err := template.New(name).Parse(string(jsonB))
		if err != nil {
			return fmt.Errorf("while parsing template for service %s: %w", name, err)
		}

		buf := &bytes.Buffer{}
		if err := t.Execute(buf, c.Data); err != nil {
			return fmt.Errorf("while executing template for service %s: %w", name, err)
		}

		newSvc := map[string]any{}
		if err := jsonCodec.Unmarshal(buf.Bytes(), &newSvc); err != nil {
			return fmt.Errorf("while unmarshalling service %s: %w", name, err)
		}

		c.Data["services"].(map[string]any)[name] = newSvc //nolint:errcheck
	}

	return nil
}
