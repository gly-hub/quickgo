package quickgo

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConfigLoaderAllowsUnrelatedKeys(t *testing.T) {
	dir := t.TempDir()
	content := []byte("app:\n  name: quickgo\n  unknown: true\n")
	if err := os.WriteFile(filepath.Join(dir, "configs_local.yaml"), content, 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	loader, err := NewConfigLoader(EnvLocal, dir)
	if err != nil {
		t.Fatalf("NewConfigLoader failed: %v", err)
	}
	var config struct {
		App struct {
			Name string `yaml:"name"`
		} `yaml:"app"`
	}
	if err := loader.Load(&config); err != nil {
		t.Fatalf("Load failed for a partial config: %v", err)
	}
	if config.App.Name != "quickgo" {
		t.Fatalf("unexpected config: %#v", config)
	}
}

func TestConfigLoaderLoadKeyAllowsUnrelatedKeys(t *testing.T) {
	dir := t.TempDir()
	content := []byte("app:\n  name: quickgo\n  unknown: true\n")
	if err := os.WriteFile(filepath.Join(dir, "configs_local.yaml"), content, 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	loader, err := NewConfigLoader(EnvLocal, dir)
	if err != nil {
		t.Fatalf("NewConfigLoader failed: %v", err)
	}
	var app struct {
		Name string `yaml:"name"`
	}
	if err := loader.LoadKey("app", &app); err != nil {
		t.Fatalf("LoadKey failed for a partial config: %v", err)
	}
	if app.Name != "quickgo" {
		t.Fatalf("unexpected config: %#v", app)
	}
}

func TestConfigLoaderLoadsMultiplePartialStructs(t *testing.T) {
	dir := t.TempDir()
	content := []byte("app:\n  name: quickgo\nhttp:\n  port: 8080\n")
	if err := os.WriteFile(filepath.Join(dir, "configs_local.yaml"), content, 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	loader, err := NewConfigLoader(EnvLocal, dir)
	if err != nil {
		t.Fatalf("NewConfigLoader failed: %v", err)
	}
	var app struct {
		App struct {
			Name string `yaml:"name"`
		} `yaml:"app"`
	}
	var http struct {
		HTTP struct {
			Port int `yaml:"port"`
		} `yaml:"http"`
	}
	if err := loader.Load(&app, &http); err != nil {
		t.Fatalf("Load multiple structs failed: %v", err)
	}
	if app.App.Name != "quickgo" || http.HTTP.Port != 8080 {
		t.Fatalf("unexpected decoded config: %#v %#v", app, http)
	}
}

func TestConfigLoaderRejectsAmbiguousFormats(t *testing.T) {
	dir := t.TempDir()
	for name, content := range map[string]string{
		"configs_local.yaml": "app:\n  name: quickgo\n",
		"configs_local.toml": "[app]\nname = 'quickgo'\n",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}

	_, err := NewConfigLoader(EnvLocal, dir)
	if err == nil || !strings.Contains(err.Error(), "multiple config files") {
		t.Fatalf("expected ambiguous config error, got %v", err)
	}
}

func TestConfigLoaderSupportsYMLExtension(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "configs_local.yml"), []byte("app:\n  name: quickgo\n"), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	loader, err := NewConfigLoader(EnvLocal, dir)
	if err != nil {
		t.Fatalf("NewConfigLoader failed: %v", err)
	}
	if got := loader.GetConfigFormat(); got != ConfigFormatYAML {
		t.Fatalf("expected canonical YAML format, got %q", got)
	}
}
