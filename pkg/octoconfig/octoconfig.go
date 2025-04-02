// Package octoconfig provides high level config handling for octoctl.
package octoconfig

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"iter"
	"log/slog"
	"maps"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"text/template"

	"dario.cat/mergo"
	"github.com/go-orb/go-orb/codecs"
	"github.com/go-orb/go-orb/config"
	"github.com/go-orb/go-orb/log"
	"github.com/octocompose/octoctl/pkg/octocache"

	"github.com/hashicorp/go-multierror"
	"github.com/lithammer/shortuuid/v3"
)

const schemeFile = "file"

const gpgAsc = ".asc"

// OctoctlConfig represents the `octoctl` structure of the octoctl config file.
type OctoctlConfig struct {
	Operator string   `json:"operator"`
	Command  []string `json:"command,omitempty"`
}

// AbsURL makes the URL absolute if it is relative.
func AbsURL(dst *url.URL, src *url.URL) {
	if !filepath.IsAbs(dst.Path) {
		dst.Scheme = src.Scheme
		dst.Host = src.Host
		dir := filepath.Dir(src.Path)
		dst.Path = filepath.Join(dir, dst.Path)
	}
}

func templateFile(url *config.URL, projectID string, templateVars map[string]any) (*config.URL, error) {
	if url.Scheme != schemeFile {
		return nil, fmt.Errorf("while templating file '%s': only 'file' URLs are supported", url.String())
	}

	sha256sum := sha256.Sum256([]byte(url.URL.String()))
	ext := filepath.Ext(url.URL.Path)

	templatePath, err := octocache.Path(projectID, "template", hex.EncodeToString(sha256sum[:16])+ext)
	if err != nil {
		return nil, err
	}

	// Remove existing file.
	if _, err := os.Stat(templatePath); err == nil {
		if err := os.Remove(templatePath); err != nil {
			return nil, fmt.Errorf("while removing file '%s': %w", templatePath, err)
		}
	}

	// Read the file.
	fpRead, err := os.Open(url.URL.Path)
	if err != nil {
		return nil, fmt.Errorf("while opening file '%s': %w", url.URL.String(), err)
	}

	defer func() {
		if err := fpRead.Close(); err != nil {
			slog.Error("failed to close template file", "path", url.URL.String(), "error", err)
		}
	}()

	b, err := io.ReadAll(fpRead)
	if err != nil {
		return nil, fmt.Errorf("while reading file '%s': %w", url.URL.String(), err)
	}

	// Write the file.
	fpWrite, err := os.OpenFile(templatePath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600) //nolint:gosec
	if err != nil {
		return nil, fmt.Errorf("while opening file '%s': %w", templatePath, err)
	}

	defer func() {
		if err := fpWrite.Close(); err != nil {
			slog.Error("failed to close template file", "path", templatePath, "error", err)
		}
	}()

	t, err := template.New("template").Parse(string(b))
	if err != nil {
		return nil, fmt.Errorf("while parsing template: %w", err)
	}

	buf := &bytes.Buffer{}
	if err := t.Execute(buf, templateVars); err != nil {
		return nil, fmt.Errorf("while executing template: %w", err)
	}

	if _, err := fpWrite.Write(buf.Bytes()); err != nil {
		return nil, fmt.Errorf("while writing file '%s': %w", templatePath, err)
	}

	result, err := config.NewURL(templatePath)
	if err != nil {
		return nil, fmt.Errorf("while creating URL '%s': %w", templatePath, err)
	}

	result.Scheme = schemeFile

	return result, nil
}

// configIncludeVersions represents the versions of a configuration include.
type configIncludeVersions struct {
	Format string      `json:"format"`
	URL    *config.URL `json:"url"`
}

// urlConfig represents a configuration include.
type urlConfig struct {
	URL      *config.URL           `json:"url"`
	GPG      *config.URL           `json:"gpg"`
	Versions configIncludeVersions `json:"versions"`

	Cached   *config.URL    `json:"-"`
	Data     map[string]any `json:"-"`
	Includes []*urlConfig   `json:"-"`
	Repo     *Repo          `json:"-"`
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
func (u *urlConfig) FlattenRepo() iter.Seq[*Repo] {
	return iter.Seq[*Repo](func(yield func(*Repo) bool) {
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
func (c *Config) readRepo(ctx context.Context, url *config.URL, parent *Repo) error {
	c.logger.Trace("Read repository", "url", url.String())

	// Resolve the URL.
	cached, err := octocache.CachedURL(ctx, c.ProjectID, url, nil, "repos", true)
	if err != nil {
		return err
	}

	// Read the cached file.
	data, err := config.Read(cached.URL)
	if err != nil {
		return err
	}

	tmpRepo := &Repo{}
	tmpRepo.URL = url

	if err := config.Parse(nil, "", data, tmpRepo); err != nil {
		return fmt.Errorf("while parsing repository '%s': %w", url.String(), err)
	}

	parent.Children = append(parent.Children, tmpRepo)

	for _, include := range tmpRepo.Include {
		// Make the URL absolute if it's a relative URL.
		AbsURL(include.URL.URL, url.URL)

		if include.GPG != nil {
			AbsURL(include.GPG.URL, url.URL)
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
		AbsURL(repo.URL.URL, fileConfig.URL.URL)

		if repo.GPG != nil {
			AbsURL(repo.GPG.URL, fileConfig.URL.URL)
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

// readURL reads the configuration data from a urlConfig's URL.
func (c *Config) readURL(ctx context.Context, fileConfig *urlConfig) error {
	mErr := &multierror.Error{}

	// Resolve the URL.
	cached, err := octocache.CachedURL(ctx, c.ProjectID, fileConfig.URL, nil, "configs", true)
	if err != nil {
		return err
	}

	c.logger.Trace("Read file", "original", fileConfig.URL.String(), "cached", cached.URL.String())

	// Read the cached file.
	data, err := config.Read(cached.URL)
	if err != nil {
		return err
	}

	fileConfig.Cached = cached
	fileConfig.Data = data

	fileConfig.Repo = &Repo{}
	fileConfig.Repo.URL = fileConfig.URL

	err = config.Parse(nil, "repos", fileConfig.Data, fileConfig.Repo)
	if err != nil {
		if !errors.Is(err, config.ErrNoSuchKey) {
			mErr = multierror.Append(mErr, fmt.Errorf("while parsing repository '%s': %w", fileConfig.URL.String(), err))
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
			mErr = multierror.Append(mErr, fmt.Errorf("while parsing includes '%s': %w", fileConfig.URL.String(), err))
		}

		return mErr.ErrorOrNil()
	}

	// Remove the include section from the data.
	delete(fileConfig.Data, "include")

	// Iterate over the include paths and create URLConfigs for them.
	for _, include := range includes {
		// Make the URL absolute if it's a relative URL.
		AbsURL(include.URL.URL, fileConfig.URL.URL)

		if include.GPG != nil {
			AbsURL(include.GPG.URL, fileConfig.URL.URL)
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
		if err := c.readURL(ctx, include); err != nil {
			mErr = multierror.Append(mErr, err)
		}
	}

	return mErr.ErrorOrNil()
}

// Config represents a configuration.
type Config struct {
	logger log.Logger

	clearCache bool

	Paths   []*urlConfig
	Repo    *Repo
	Octoctl *OctoctlConfig

	KnownURLs map[string]struct{}

	// Hardcoded config
	HardcodedData map[string]any

	// Merged data.
	Data map[string]any

	ProjectID string
}

// New creates a new configuration from the given paths.
func New(logger log.Logger, clearCache bool, paths []string, hardcodedData map[string]any) (*Config, error) {
	cfg := &Config{logger: logger, Paths: []*urlConfig{}, KnownURLs: map[string]struct{}{}, Repo: &Repo{}, HardcodedData: hardcodedData}

	for _, path := range paths {
		myURL, err := config.NewURL(path)
		if err != nil {
			return nil, err
		}

		if myURL.Scheme == "" {
			myURL.Scheme = schemeFile

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

	cfg.HardcodedData = hardcodedData
	cfg.clearCache = clearCache

	return cfg, nil
}

// Run runs the configuration.
func (c *Config) Run(ctx context.Context) error {
	if err := c.ensureProjectID(ctx); err != nil {
		return err
	}

	if c.clearCache {
		if err := octocache.ClearCache(c.ProjectID); err != nil {
			return err
		}
	}

	if err := c.read(ctx); err != nil {
		return err
	}

	if err := c.merge(ctx); err != nil {
		return err
	}

	if err := c.applyGlobals(); err != nil {
		return err
	}

	if err := c.processFileTemplates(ctx); err != nil {
		return err
	}

	if err := c.mergeRepos(ctx); err != nil {
		return err
	}

	if err := c.applyServiceTemplates(); err != nil {
		return err
	}

	return nil
}

// ensureProjectID ensures that the projectID is set in the configuration.
func (c *Config) ensureProjectID(_ context.Context) error {
	for _, cfg := range c.Paths {
		data, err := config.Read(cfg.URL.URL)
		if err != nil {
			// If we can't read the config, we assume it doesn't have a projectID.
			continue
		}

		projectID, ok := data["name"]
		if !ok {
			continue
		}

		c.ProjectID, ok = projectID.(string)
		if !ok {
			continue
		}

		c.logger.Debug("Using name from config", "name", c.ProjectID)

		return nil
	}

	if c.ProjectID == "" {
		// Generate a projectID if none was found.
		c.ProjectID = shortuuid.New()
		c.logger.Debug("Generated a new name", "name", c.ProjectID)
	}

	return nil
}

// read reads the configuration data from all paths.
func (c *Config) read(ctx context.Context) error {
	mErr := &multierror.Error{}

	for _, path := range c.Paths {
		if err := c.readURL(ctx, path); err != nil {
			mErr = multierror.Append(mErr, err)
		}
	}

	return mErr.ErrorOrNil()
}

// mergeRepos reads and merges repos from the config(s).
func (c *Config) mergeRepos(_ context.Context) error {
	mErr := &multierror.Error{}

	repoFiles := []*Repo{}
	for _, path := range c.Paths {
		repoFiles = append(repoFiles, slices.Collect(path.FlattenRepo())...)
	}

	slices.Reverse(repoFiles)

	// Merge all repo files.
	for _, repoFile := range repoFiles {
		repoFile.Include = nil

		if err := mergo.Merge(c.Repo, repoFile, mergo.WithOverride, mergo.WithAppendSlice); err != nil {
			mErr = multierror.Append(mErr, err)
			continue
		}
	}

	// Merge the repos from the config file.
	repoFile := &Repo{}
	if err := config.Parse(nil, "repos", c.Data, repoFile); err == nil {
		mergo.Merge(c.Repo, repoFile, mergo.WithOverride, mergo.WithAppendSlice)
	}

	data, err := config.ParseStruct(nil, c.Repo)

	if err != nil {
		mErr = multierror.Append(mErr, fmt.Errorf("while parsing repos: %w", err))
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

// merge merges the configuration data from all paths.
func (c *Config) merge(_ context.Context) error {
	// Collect configurations in the correct order for merging.
	configs := c.collectConfigs()

	var mErr *multierror.Error

	// Merge hardcoded data first.
	if err := mergo.Merge(&c.Data, c.HardcodedData, mergo.WithOverride, mergo.WithAppendSlice); err != nil {
		mErr = multierror.Append(mErr, err)
	}

	for _, cfg := range configs {
		// Log that we're merging this config.
		c.logger.Trace("Merging config", "url", cfg.URL.String())

		if err := mergo.Merge(&c.Data, cfg.Data, mergo.WithOverride, mergo.WithAppendSlice); err != nil {
			mErr = multierror.Append(mErr, err)
		}
	}

	// Load octoctl config.
	c.Octoctl = &OctoctlConfig{}
	if err := config.Parse([]string{}, "octoctl", c.Data, c.Octoctl); err != nil {
		mErr = multierror.Append(mErr, fmt.Errorf("while parsing octoctl: %w", err))
	}

	return mErr.ErrorOrNil()
}

// svcConfig represents a service configuration.
type svcConfig struct {
	Globals    string `json:"globals"`
	NoTemplate bool   `json:"noTemplate"`
}

// applyGlobals applies the global configuration to each service.
func (c *Config) applyGlobals() error {
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

		if err := mergo.Merge(&mergedConfig, svcGlobal, mergo.WithOverride, mergo.WithAppendSlice); err != nil {
			return err
		}

		// Then merge in the service configuration so it takes precedence.
		svcConfig, ok := servicesConfig[name].(map[string]any)
		if ok {
			if err := mergo.Merge(&mergedConfig, svcConfig, mergo.WithOverride, mergo.WithAppendSlice); err != nil {
				return err
			}
		}

		servicesConfig[name] = mergedConfig
	}

	c.Data["configs"] = servicesConfig

	delete(c.Data, "globals")

	return nil
}

// TemplateVars returns the template variables.
func (c *Config) TemplateVars() map[string]any {
	data := maps.Clone(c.Data)

	// Add project ID.
	data["projectID"] = c.ProjectID

	// Add OS and ARCH variables.
	data["OS"] = runtime.GOOS
	data["ARCH"] = runtime.GOARCH

	// Add environment variables.
	envData := map[string]any{}

	for _, env := range os.Environ() {
		split := strings.SplitN(env, "=", 2)
		envData[split[0]] = split[1]
	}

	data["env"] = envData

	return data
}

// applyServiceTemplates applies the service templates.
func (c *Config) applyServiceTemplates() error {
	var services map[string]any

	if err := config.Parse([]string{}, "services", c.Data, &services); err != nil {
		if errors.Is(err, config.ErrNoSuchKey) {
			return nil
		}

		return fmt.Errorf("while parsing services: %w", err)
	}

	yamlCodec, err := codecs.GetMime(codecs.MimeYAML)
	if err != nil {
		return fmt.Errorf("while getting YAML codec: %w", err)
	}

	templateVars := c.TemplateVars()

	for name, svc := range services {
		yamlB, err := yamlCodec.Marshal(svc)
		if err != nil {
			return fmt.Errorf("while marshaling service %s: %w", name, err)
		}

		t, err := template.New(name).Parse(string(yamlB))
		if err != nil {
			return fmt.Errorf("while parsing template for service %s: %w", name, err)
		}

		buf := &bytes.Buffer{}
		if err := t.Execute(buf, templateVars); err != nil {
			return fmt.Errorf("while executing template for service %s: %w", name, err)
		}

		newSvc := map[string]any{}
		if err := yamlCodec.Unmarshal(buf.Bytes(), &newSvc); err != nil {
			return fmt.Errorf("while unmarshalling service %s: %w", name, err)
		}

		c.Data["services"].(map[string]any)[name] = newSvc
	}

	return nil
}

// processFileTemplates processes templates in files.
func (c *Config) processFileTemplates(ctx context.Context) error {
	mErr := &multierror.Error{}

	templateVars := c.TemplateVars()

	repoFiles := []*Repo{}
	for _, path := range c.Paths {
		repoFiles = append(repoFiles, slices.Collect(path.FlattenRepo())...)
	}

	slices.Reverse(repoFiles)

	for _, repo := range repoFiles {
		for name, operator := range repo.Operators {
			if operator.Source.Path != nil {
				AbsURL(operator.Source.Path.URL, repo.URL.URL)
			}

			repo.Operators[name] = operator
		}

		for fileName, fileValue := range repo.Files {
			if fileValue.URL == nil {
				continue
			}

			AbsURL(fileValue.URL.URL, repo.URL.URL)

			cached, err := octocache.CachedURL(ctx, c.ProjectID, fileValue.URL, nil, "files", true)
			if err != nil {
				mErr = multierror.Append(mErr, err)
				continue
			}

			fileValue.URL = cached
			fileValue.URL.Scheme = "file"
			repo.Files[fileName] = fileValue

			if !fileValue.Template {
				fileValue.Path = cached.URL.Path
				repo.Files[fileName] = fileValue
				continue
			}

			templateURL, err := templateFile(fileValue.URL, c.ProjectID, templateVars)
			if err != nil {
				mErr = multierror.Append(mErr, err)
				continue
			}

			fileValue.Path = templateURL.Path
			repo.Files[fileName] = fileValue
		}
	}

	return mErr.ErrorOrNil()
}
