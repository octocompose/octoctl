// Package octoconfig provides high level config handling for octoctl.
package octoconfig

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
	"text/template"

	"dario.cat/mergo"
	"github.com/go-orb/go-orb/codecs"
	"github.com/go-orb/go-orb/config"
	"github.com/go-orb/go-orb/log"

	"github.com/hashicorp/go-multierror"
	"github.com/lithammer/shortuuid/v3"
)

const gpgAsc = ".asc"

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

// absURL makes the URL absolute if it is relative.
func absURL(dst *url.URL, src *url.URL) {
	if !filepath.IsAbs(dst.Path) {
		dst.Scheme = src.Scheme
		dst.Host = src.Host
		dir := filepath.Dir(src.Path)
		dst.Path = filepath.Join(dir, dst.Path)
	}
}

// cachedURL returns a cached version of the given URL.
func cachedURL(ctx context.Context, projectID string, url *JURL, cacheType string) (*JURL, error) {
	if url.Scheme == "file" {
		return url, nil
	}

	userCacheDir, err := os.UserCacheDir()
	if err != nil {
		return nil, err
	}

	sha256sum := sha256.Sum256([]byte(url.URL.String()))
	ext := filepath.Ext(url.URL.Path)

	cachedPath := filepath.Join(userCacheDir, "octoctl", projectID, cacheType, string(sha256sum[:])+ext)
	if err := os.MkdirAll(filepath.Dir(cachedPath), 0700); err != nil {
		return nil, fmt.Errorf("while creating the cache directory '%s': %w", filepath.Dir(cachedPath), err)
	}

	// Check and return if the file already exists.
	if _, err := os.Stat(cachedPath); err == nil {
		return NewJURL(cachedPath)
	}

	if err := downloadFile(ctx, cachedPath, url.URL); err != nil {
		return nil, fmt.Errorf("while downloading config '%s': %w", url.URL.String(), err)
	}

	return NewJURL(cachedPath)
}

// configIncludeVersions represents the versions of a configuration include.
type configIncludeVersions struct {
	Format string `json:"format"`
	URL    *JURL  `json:"url"`
}

// urlConfig represents a configuration include.
type urlConfig struct {
	URL      *JURL                 `json:"url"`
	GPG      *JURL                 `json:"gpg"`
	Versions configIncludeVersions `json:"versions"`

	Cached   *JURL          `json:"-"`
	Data     map[string]any `json:"-"`
	Includes []*urlConfig   `json:"-"`
	Repo     *RepoFile      `json:"-"`
}

// Flatten returns a sequence iterator that yields the urlConfig and all its includes.
func (u *urlConfig) Flatten() iter.Seq[*urlConfig] {
	return iter.Seq[*urlConfig](func(yield func(*urlConfig) bool) {
		if !yield(u) {
			return
		}

		for _, include := range u.Includes {
			for subConfig := range include.Flatten() {
				if !yield(subConfig) {
					return
				}
			}
		}
	})
}

// FlattenRepo returns a sequence iterator that yields the *RepoFile and all its children.
func (u *urlConfig) FlattenRepo() iter.Seq[*RepoFile] {
	return iter.Seq[*RepoFile](func(yield func(*RepoFile) bool) {
		for uConfig := range u.Flatten() {
			for child := range uConfig.Repo.Flatten() {
				if !yield(child) {
					return
				}
			}
		}
	})
}

func (u *urlConfig) String() string {
	return u.URL.URL.String()
}

// readRepo reads a repository configuration file.
func (c *Config) readRepo(ctx context.Context, url *JURL, parent *RepoFile) error {
	c.logger.Debug("Read repository", "url", url.String())

	// Resolve the URL.
	cached, err := cachedURL(ctx, c.ProjectID, url, "repos")
	if err != nil {
		return err
	}

	// Read the cached file.
	data, err := config.Read(cached.URL)
	if err != nil {
		return err
	}

	tmpRepo := &RepoFile{}
	tmpRepo.URL = url

	if err := config.Parse(nil, "", data, tmpRepo); err != nil {
		return err
	}

	parent.Children = append(parent.Children, tmpRepo)

	for _, include := range tmpRepo.Include {
		// Make the URL absolute if it's a relative URL.
		absURL(include.URL.URL, url.URL)

		if include.GPG != nil {
			absURL(include.GPG.URL, url.URL)
		} else {
			gpg, err := include.URL.Copy()
			if err != nil {
				return err
			}

			gpg.URL.Path += gpgAsc
			include.GPG = gpg
		}

		if err := c.readRepo(ctx, include.URL, tmpRepo); err != nil {
			return err
		}

		tmpRepo.Include = nil
	}

	return nil
}

func (c *Config) processFileRepo(ctx context.Context, fileConfig *urlConfig) error {
	mErr := &multierror.Error{}

	for _, repo := range fileConfig.Repo.Include {
		// Make the URL absolute if it's a relative URL.
		absURL(repo.URL.URL, fileConfig.URL.URL)

		if repo.GPG != nil {
			absURL(repo.GPG.URL, fileConfig.URL.URL)
		} else {
			gpg, err := repo.URL.Copy()
			if err != nil {
				mErr = multierror.Append(mErr, err)
				continue
			}

			gpg.URL.Path += gpgAsc
			repo.GPG = gpg
		}

		if err := c.readRepo(ctx, repo.URL, fileConfig.Repo); err != nil {
			mErr = multierror.Append(mErr, err)
		}
	}

	fileConfig.Repo.Include = nil

	delete(fileConfig.Data, "repos")

	return mErr.ErrorOrNil()
}

// ReadURL reads the configuration data from a urlConfig's URL.
func (c *Config) ReadURL(ctx context.Context, fileConfig *urlConfig) error {
	mErr := &multierror.Error{}

	// Resolve the URL.
	cached, err := cachedURL(ctx, c.ProjectID, fileConfig.URL, "configs")
	if err != nil {
		return err
	}

	// Read the cached file.
	data, err := config.Read(cached.URL)
	if err != nil {
		return err
	}

	fileConfig.Cached = cached
	fileConfig.Data = data

	fileConfig.Repo = &RepoFile{}
	fileConfig.Repo.URL = fileConfig.URL

	err = config.Parse(nil, "repos", fileConfig.Data, fileConfig.Repo)
	if err != nil {
		if !errors.Is(err, config.ErrNoSuchKey) {
			mErr = multierror.Append(mErr, err)
		}
	}

	if err == nil {
		err = c.processFileRepo(ctx, fileConfig)
		if err != nil {
			mErr = multierror.Append(mErr, err)
		}
	}

	// Recursively parse the include section if present.
	var includes []*urlConfig

	if err := config.ParseSlice([]string{}, "include", fileConfig.Data, &includes); err != nil {
		if !errors.Is(err, config.ErrNoSuchKey) {
			mErr = multierror.Append(mErr, err)
		}

		return mErr.ErrorOrNil()
	}

	// Remove the include section from the data.
	delete(fileConfig.Data, "include")

	// Iterate over the include paths and create URLConfigs for them.
	for _, include := range includes {
		// Make the URL absolute if it's a relative URL.
		absURL(include.URL.URL, fileConfig.URL.URL)

		if include.GPG != nil {
			absURL(include.GPG.URL, fileConfig.URL.URL)
		} else {
			gpg, err := include.URL.Copy()
			if err != nil {
				mErr = multierror.Append(mErr, err)
				continue
			}

			gpg.URL.Path += gpgAsc
			include.GPG = gpg
		}

		// Add the include to the URLConfig.
		fileConfig.Includes = append(fileConfig.Includes, include)

		// Read the include.
		if err := c.ReadURL(ctx, include); err != nil {
			mErr = multierror.Append(mErr, err)
		}
	}

	return mErr.ErrorOrNil()
}

// Config represents a configuration.
type Config struct {
	logger log.Logger

	Paths []*urlConfig
	Repos *RepoFile

	KnownURLs map[string]struct{}

	// Merged data.
	Data map[string]any

	ProjectID string
}

// New creates a new configuration from the given paths.
func New(logger log.Logger, paths []string) (*Config, error) {
	cfg := &Config{logger: logger, Paths: []*urlConfig{}, KnownURLs: map[string]struct{}{}, Repos: &RepoFile{}}

	for _, path := range paths {
		myURL, err := NewJURL(path)
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
			cfg.logger.Warn("duplicate URL", "url", myURL.String())
			continue
		}

		cfg.Paths = append(cfg.Paths, &urlConfig{URL: myURL})
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

	if err := c.MergeRepos(ctx); err != nil {
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
			// If we can't read the config, we assume it doesn't have a projectID.
			continue
		}

		projectID, ok := data["projectID"]
		if !ok {
			continue
		}

		c.ProjectID, ok = projectID.(string)
		if !ok {
			continue
		}

		c.logger.Debug("Using project ID from config", "projectID", c.ProjectID)

		return nil
	}

	if c.ProjectID == "" {
		// Generate a projectID if none was found.
		c.ProjectID = shortuuid.New()
		c.logger.Debug("Generated new project ID", "projectID", c.ProjectID)
	}

	return nil
}

// Read reads the configuration data from all paths.
func (c *Config) Read(ctx context.Context) error {
	mErr := &multierror.Error{}

	for _, path := range c.Paths {
		if err := c.ReadURL(ctx, path); err != nil {
			mErr = multierror.Append(mErr, err)
		}
	}

	return mErr.ErrorOrNil()
}

// MergeRepos reads and merges repos from the config(s).
func (c *Config) MergeRepos(ctx context.Context) error {
	mErr := &multierror.Error{}

	repoFiles := []*RepoFile{}
	for _, path := range c.Paths {
		repoFiles = append(repoFiles, slices.Collect(path.FlattenRepo())...)
	}

	slices.Reverse(repoFiles)

	for _, repoFile := range repoFiles {
		repoFile.Include = nil

		if err := c.processRepoFileURLs(ctx, repoFile); err != nil {
			mErr = multierror.Append(mErr, err)
			continue
		}

		if err := mergo.Merge(c.Repos, repoFile); err != nil {
			mErr = multierror.Append(mErr, err)
			continue
		}
	}

	data, err := config.ParseStruct(nil, c.Repos)

	if err != nil {
		mErr = multierror.Append(mErr, err)
		return mErr.ErrorOrNil()
	}

	if c.Data == nil {
		c.Data = make(map[string]any)
	}

	c.Data["repos"] = data

	return mErr.ErrorOrNil()
}

// collectConfigs collects all configurations in the proper processing order.
func (c *Config) collectConfigs() []*urlConfig {
	var configs []*urlConfig

	// Process configs in path order.
	for _, path := range c.Paths {
		configs = append(configs, slices.Collect(path.Flatten())...)
	}

	// Reverse for proper merge priority (first defined has higher priority).
	slices.Reverse(configs)

	return configs
}

// Merge merges the configuration data from all paths.
func (c *Config) Merge(_ context.Context) error {
	// Collect configurations in the correct order for merging.
	configs := c.collectConfigs()

	var mErr *multierror.Error

	for _, cfg := range configs {
		// Log that we're merging this config.
		c.logger.Debug("Merging config", "path", cfg.URL.String())

		if err := config.Merge(&c.Data, cfg.Data); err != nil {
			mErr = multierror.Append(mErr, err)
		}
	}

	return mErr.ErrorOrNil()
}

// svcConfig represents a service configuration.
type svcConfig struct {
	Globals    string `json:"globals"`
	NoTemplate bool   `json:"noTemplate"`
}

// ApplyGlobals applies the global configuration to each service.
func (c *Config) ApplyGlobals() error {
	services := map[string]any{}

	if err := config.Parse([]string{}, "services", c.Data, &services); err != nil {
		if errors.Is(err, config.ErrNoSuchKey) {
			return nil
		}

		return err
	}

	servicesConfig := map[string]any{}
	globalsConfig := map[string]any{}

	if err := config.Parse([]string{}, "globals", c.Data, &globalsConfig); err != nil {
		if !errors.Is(err, config.ErrNoSuchKey) {
			return err
		}

		return nil
	}

	if err := config.Parse([]string{}, "configs", c.Data, &servicesConfig); err != nil {
		if !errors.Is(err, config.ErrNoSuchKey) {
			return err
		}
	}

	for name := range services {
		servicesSvcConfig := svcConfig{}

		if err := config.Parse([]string{"services", name, "octocompose"}, "config", c.Data, &servicesSvcConfig); err != nil {
			continue
		}

		if servicesSvcConfig.Globals == "" {
			continue
		}

		// Parse config from globals.
		globalsConfig := globalsConfig[servicesSvcConfig.Globals]
		if globalsConfig == nil {
			c.logger.Error("Global config not found", "service", name, "global", servicesSvcConfig.Globals)
			return fmt.Errorf("service '%s' requires global config '%s', but it was not found", name, servicesSvcConfig.Globals)
		}

		// Create a merged configuration by merging globals with service configuration.
		mergedConfig := map[string]any{}

		// First copy the globals configuration.
		svcGlobal, ok := globalsConfig.(map[string]any)
		if !ok {
			c.logger.Error("Global config not found", "service", name, "global", servicesSvcConfig.Globals)
			return fmt.Errorf("service '%s' requires global config '%s', but it was not found", name, servicesSvcConfig.Globals)
		}

		if err := config.Merge(&mergedConfig, svcGlobal); err != nil {
			return err
		}

		// Then merge in the service configuration so it takes precedence.
		svcConfig, ok := servicesConfig[name].(map[string]any)
		if ok {
			if err := config.Merge(&mergedConfig, svcConfig); err != nil {
				return err
			}
		}

		servicesConfig[name] = mergedConfig
	}

	c.Data["configs"] = servicesConfig

	delete(c.Data, "globals")

	return nil
}

// ApplyServiceTemplates applies the service templates.
func (c *Config) ApplyServiceTemplates() error {
	var services map[string]any

	if err := config.Parse([]string{}, "services", c.Data, &services); err != nil {
		if errors.Is(err, config.ErrNoSuchKey) {
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

		c.Data["services"].(map[string]any)[name] = newSvc
	}

	return nil
}

// processRepoFileURLs processes URLs in files and makes them absolute.
func (c *Config) processRepoFileURLs(ctx context.Context, repo *RepoFile) error {
	mErr := &multierror.Error{}

	for fileName, fileValue := range repo.Files {
		if fileValue.URL == nil {
			continue
		}

		absURL(fileValue.URL.URL, repo.URL.URL)

		cached, err := cachedURL(ctx, c.ProjectID, fileValue.URL, "files")
		if err != nil {
			mErr = multierror.Append(mErr, err)
			continue
		}

		fileValue.Path = cached.Path
		repo.Files[fileName] = fileValue
	}

	return mErr.ErrorOrNil()
}
