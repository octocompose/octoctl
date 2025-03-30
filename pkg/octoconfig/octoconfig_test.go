package octoconfig

import (
	"path/filepath"
	"testing"

	"github.com/go-orb/go-orb/config"
	"github.com/go-orb/go-orb/log"

	"github.com/stretchr/testify/require"

	_ "github.com/go-orb/plugins/codecs/json"
	_ "github.com/go-orb/plugins/codecs/toml"
	_ "github.com/go-orb/plugins/codecs/yaml"
	_ "github.com/go-orb/plugins/config/source/file"
	_ "github.com/go-orb/plugins/log/slog"
)

func setupTestConfig() *Config {
	// Create a test Config with all required fields
	logger, err := log.New(log.WithLevel(log.LevelDebug))
	if err != nil {
		panic(err)
	}
	return &Config{
		ProjectID: "test",
		Paths:     []*urlConfig{},
		KnownURLs: make(map[string]struct{}),
		Data:      make(map[string]any),
		logger:    logger,
	}
}

func TestCollectConfigs(t *testing.T) {
	// Create a Config with multiple paths
	cfg := setupTestConfig()

	// Create test URLConfigs
	url1, err := config.NewURL("file:///path/to/config1.json")
	require.NoError(t, err)
	url11, err := config.NewURL("file:///path/to/config11.json")
	require.NoError(t, err)
	url111, err := config.NewURL("file:///path/to/config111.json")
	require.NoError(t, err)
	url112, err := config.NewURL("file:///path/to/config112.json")
	require.NoError(t, err)

	url2, err := config.NewURL("file:///path/to/config2.json")
	require.NoError(t, err)
	url21, err := config.NewURL("file:///path/to/config21.json")
	require.NoError(t, err)
	url211, err := config.NewURL("file:///path/to/config211.json")
	require.NoError(t, err)
	url212, err := config.NewURL("file:///path/to/config212.json")
	require.NoError(t, err)

	config111 := &urlConfig{
		URL:  url111,
		Data: map[string]any{"key111": "value111"},
	}

	config112 := &urlConfig{
		URL:  url112,
		Data: map[string]any{"key112": "value112"},
	}

	config11 := &urlConfig{
		URL:      url11,
		Data:     map[string]any{"key11": "value11"},
		Includes: []*urlConfig{config111, config112},
	}

	config1 := &urlConfig{
		URL:      url1,
		Data:     map[string]any{"key1": "value1"},
		Includes: []*urlConfig{config11},
	}

	config211 := &urlConfig{
		URL:  url211,
		Data: map[string]any{"key211": "value211"},
	}

	config212 := &urlConfig{
		URL:  url212,
		Data: map[string]any{"key212": "value212"},
	}

	config21 := &urlConfig{
		URL:      url21,
		Data:     map[string]any{"key21": "value21"},
		Includes: []*urlConfig{config211, config212},
	}

	config2 := &urlConfig{
		URL:      url2,
		Data:     map[string]any{"key2": "value2"},
		Includes: []*urlConfig{config21},
	}

	cfg.Paths = append(cfg.Paths, config1, config2)

	// Test collectConfigs method
	configs := cfg.collectConfigs()

	// Check that we have all configs in the right order
	require.Len(t, configs, 8)
	require.Equal(t, config212.String(), configs[0].String())
	require.Equal(t, config211.String(), configs[1].String())
	require.Equal(t, config21.String(), configs[2].String())
	require.Equal(t, config2.String(), configs[3].String())
	require.Equal(t, config112.String(), configs[4].String())
	require.Equal(t, config111.String(), configs[5].String())
	require.Equal(t, config11.String(), configs[6].String())
	require.Equal(t, config1.String(), configs[7].String())
}

func TestMerge(t *testing.T) {
	// Create a test Config
	cfg := setupTestConfig()

	// Create test URLConfigs
	url1, err := config.NewURL("file:///path/to/config1.json")
	require.NoError(t, err)
	url2, err := config.NewURL("file:///path/to/config2.json")
	require.NoError(t, err)

	config1 := &urlConfig{
		URL:  url1,
		Data: map[string]any{"key1": "value1", "shared": "value1"},
	}

	config2 := &urlConfig{
		URL:  url2,
		Data: map[string]any{"key2": "value2", "shared": "value2"},
	}

	cfg.Paths = append(cfg.Paths, config1, config2)

	// Test Merge method
	err = cfg.merge(t.Context())
	require.NoError(t, err)

	// Check merged data (should be in reverse order for priority)
	require.Equal(t, "value1", cfg.Data["key1"])
	require.Equal(t, "value2", cfg.Data["key2"])
	// First defined config (config1) should have priority
	require.Equal(t, "value1", cfg.Data["shared"])
}

func TestApplyGlobals(t *testing.T) {
	// Create a test Config
	config := setupTestConfig()
	config.Data = map[string]any{
		"configs": map[string]any{
			"service1": map[string]any{
				"option1": "config_value1",
				"option3": "config_option3",
			},
		},
		"services": map[string]any{
			"service1": map[string]any{
				"octocompose": map[string]any{
					"config": map[string]any{
						"globals": "global1",
					},
				},
			},
		},
		"globals": map[string]any{
			"global1": map[string]any{
				"option1": "global_value",
				"option2": "global_option2",
			},
		},
	}

	// Test ApplyGlobals method
	err := config.applyGlobals()
	require.NoError(t, err)

	// Check that globals were applied
	configs, ok := config.Data["configs"].(map[string]any)
	require.True(t, ok, "configs should be a map[string]any")
	service1Config, ok := configs["service1"].(map[string]any)
	require.True(t, ok, "service1 should be a map[string]any")

	// Service option should override global
	require.Equal(t, "config_value1", service1Config["option1"])
	// Global option should be added
	require.Equal(t, "global_option2", service1Config["option2"])
	// Service option should be kept
	require.Equal(t, "config_option3", service1Config["option3"])
}

func TestApplyGlobalsWithNoServiceConfig(t *testing.T) {
	// Create a test Config
	config := setupTestConfig()
	config.Data = map[string]any{
		"services": map[string]any{
			"service1": map[string]any{
				"octocompose": map[string]any{
					"config": map[string]any{
						"globals": "global1",
					},
				},
			},
		},
		"globals": map[string]any{
			"global1": map[string]any{
				"option1": "global_value",
				"option2": "global_option2",
			},
		},
	}

	// Test ApplyGlobals method
	err := config.applyGlobals()
	require.NoError(t, err)

	// Check that globals were applied
	configs, ok := config.Data["configs"].(map[string]any)
	require.True(t, ok, "configs should be a map[string]any")
	service1Config, ok := configs["service1"].(map[string]any)
	require.True(t, ok, "service1 should be a map[string]any")

	require.Equal(t, "global_value", service1Config["option1"])
	require.Equal(t, "global_option2", service1Config["option2"])
}

func TestFlatten(t *testing.T) {
	// Create a temporary directory for testing
	tempDir := t.TempDir()

	// Create a test Config
	cfg := setupTestConfig()

	// Create a nested URL config structure
	mainConfigPath := filepath.Join(tempDir, "main.json")
	mainURL, err := config.NewURL("file://" + mainConfigPath)
	require.NoError(t, err)
	mainConfig := &urlConfig{
		URL:  mainURL,
		Data: map[string]any{"main": "value"},
	}

	includeConfigPath := filepath.Join(tempDir, "include.json")
	includeURL, err := config.NewURL("file://" + includeConfigPath)
	require.NoError(t, err)
	includeConfig := &urlConfig{
		URL:  includeURL,
		Data: map[string]any{"include": "value"},
	}

	mainConfig.Includes = append(mainConfig.Includes, includeConfig)
	cfg.Paths = append(cfg.Paths, mainConfig)

	// Test Flatten method by collecting all items from the Seq
	flattenedSeq := mainConfig.Flatten()
	flattened := make([]*urlConfig, 0)
	for config := range flattenedSeq {
		flattened = append(flattened, config)
	}

	require.Len(t, flattened, 2)

	// The first one should be the main config
	require.Equal(t, mainConfig, flattened[0])
	// The second one should be the include config
	require.Equal(t, includeConfig, flattened[1])
}
