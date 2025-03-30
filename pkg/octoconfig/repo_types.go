package octoconfig

import (
	"iter"

	"github.com/go-orb/go-orb/config"
)

// Repo represents the top-level structure of the repository file.
type Repo struct {
	Include   []RepoInclude            `json:"include,omitempty"`
	Files     map[string]RepoFileEntry `json:"files,omitempty"`
	Operators map[string]RepoBaremetal `json:"operators,omitempty"`
	Tools     map[string]RepoTool      `json:"tools,omitempty"`
	Services  map[string]RepoService   `json:"services,omitempty"`

	URL      *config.URL `json:"-"`
	Children []*Repo     `json:"-"`
}

// Flatten returns a sequence iterator that yields the urlConfig and all its includes.
func (r *Repo) Flatten() iter.Seq[*Repo] {
	return iter.Seq[*Repo](func(yield func(*Repo) bool) {
		if !yield(r) {
			return
		}

		for _, include := range r.Children {
			for subConfig := range include.Flatten() {
				if !yield(subConfig) {
					return
				}
			}
		}
	})
}

// RepoInclude represents a repository include.
type RepoInclude struct {
	URL *config.URL `json:"url"`
	GPG *config.URL `json:"gpg"`
}

// RepoFileEntry represents a file entry in the repository.
type RepoFileEntry struct {
	URL      *config.URL `json:"url"`
	Path     string      `json:"path"`
	Template bool        `json:"template"`
}

// RepoService represents the repository for a service.
type RepoService struct {
	Baremetal *RepoBaremetal `json:"baremetal,omitempty"`
	Docker    *RepoDocker    `json:"docker,omitempty"`
}

// RepoTool represents the repository for a tool.
type RepoTool struct {
	Baremetal *RepoBaremetal `json:"baremetal,omitempty"`
	Docker    *RepoDocker    `json:"docker,omitempty"`
}

// RepoBaremetal represents a platform-specific repository.
type RepoBaremetal struct {
	Binary map[string]RepoBinaryDistribution `json:"binary,omitempty"`
	Source *RepoSource                       `json:"source,omitempty"`
}

// RepoBinaryDistribution represents a binary distribution for a specific architecture.
type RepoBinaryDistribution struct {
	Path      *config.URL `json:"path"`
	URL       *config.URL `json:"url"`
	SHA256URL *config.URL `json:"sha256Url"`
	Binary    string      `json:"binary"`
}

// RepoSource represents the source code repository.
type RepoSource struct {
	Path      *config.URL `json:"path"`
	Repo      *config.URL `json:"repo"`
	Ref       string      `json:"ref"`
	BuildCmds []string    `json:"buildCmds"`
	Binary    string      `json:"binary"`
}

// RepoDocker represents the docker-specific repository.
type RepoDocker struct {
	Registry   string           `json:"registry"`
	Image      string           `json:"image"`
	Tag        string           `json:"tag"`
	Entrypoint string           `json:"entrypoint,omitempty"`
	Command    []string         `json:"command,omitempty"`
	Build      *RepoDockerBuild `json:"build,omitempty"`
}

// RepoDockerBuild represents the docker build repository.
type RepoDockerBuild struct {
	Repo       *config.URL `json:"repo"`
	Ref        string      `json:"ref"`
	Dockerfile string      `json:"dockerfile"`
	Context    string      `json:"context"`
}
