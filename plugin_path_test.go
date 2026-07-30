package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveDefaultDataPathFromPluginModule(t *testing.T) {
	root := filepath.Join(t.TempDir(), "CLIProxyAPI")
	modulePath := filepath.Join(root, "plugins", "darwin", "arm64", "cap-token-usage-tracker.dylib")

	got := resolveDefaultDataPath(modulePath, "", "")
	want := filepath.Join(root, "data", defaultDataFileName)
	if got != want {
		t.Fatalf("default data path = %q, want %q", got, want)
	}
}

func TestResolveDefaultDataPathFromRelativePluginModule(t *testing.T) {
	root := filepath.Join(t.TempDir(), "CLIProxyAPI")
	modulePath := filepath.Join("plugins", "linux", "amd64", "cap-token-usage-tracker.so")

	got := resolveDefaultDataPath(modulePath, "", root)
	want := filepath.Join(root, "data", defaultDataFileName)
	if got != want {
		t.Fatalf("default data path = %q, want %q", got, want)
	}
}

func TestResolveDefaultDataPathFromExecutableDirectory(t *testing.T) {
	root := filepath.Join(t.TempDir(), "CLIProxyAPI")
	if err := os.MkdirAll(filepath.Join(root, "plugins"), 0o700); err != nil {
		t.Fatal(err)
	}

	got := resolveDefaultDataPath("", filepath.Join(root, "CLIProxyAPI"), "")
	want := filepath.Join(root, "data", defaultDataFileName)
	if got != want {
		t.Fatalf("default data path = %q, want %q", got, want)
	}
}

func TestResolveDefaultDataPathFromWorkingDirectory(t *testing.T) {
	root := filepath.Join(t.TempDir(), "CLIProxyAPI")
	if err := os.MkdirAll(filepath.Join(root, "plugins"), 0o700); err != nil {
		t.Fatal(err)
	}

	got := resolveDefaultDataPath("", "", root)
	want := filepath.Join(root, "data", defaultDataFileName)
	if got != want {
		t.Fatalf("default data path = %q, want %q", got, want)
	}
}

func TestResolveDefaultDataPathFallsBackForUnknownLayout(t *testing.T) {
	base := t.TempDir()
	got := resolveDefaultDataPath(
		filepath.Join(base, "cache", "cap-token-usage-tracker.dylib"),
		filepath.Join(base, "bin", "CLIProxyAPI"),
		base,
	)
	if got != legacyDefaultDataPath {
		t.Fatalf("default data path = %q, want legacy fallback %q", got, legacyDefaultDataPath)
	}
}

func TestResolveDefaultDataPathRejectsFilesystemRoot(t *testing.T) {
	root := filepath.VolumeName(t.TempDir()) + string(filepath.Separator)
	modulePath := filepath.Join(root, "plugins", "darwin", "arm64", "cap-token-usage-tracker.dylib")

	got := resolveDefaultDataPath(modulePath, "", "")
	if got != legacyDefaultDataPath {
		t.Fatalf("default data path = %q, want legacy fallback %q", got, legacyDefaultDataPath)
	}
}

func TestParseConfigExplicitDataPathOverridesDiscoveredDefault(t *testing.T) {
	explicit := filepath.Join(t.TempDir(), "custom", "usage.db")
	config, err := parseConfig([]byte("data_path: " + filepath.ToSlash(explicit) + "\n"))
	if err != nil {
		t.Fatal(err)
	}
	if config.DataPath != explicit {
		t.Fatalf("data path = %q, want explicit path %q", config.DataPath, explicit)
	}
}

func TestParseConfigExplicitRelativeDataPathKeepsWorkingDirectorySemantics(t *testing.T) {
	relative := filepath.Join("custom", "usage.db")
	config, err := parseConfig([]byte("data_path: " + filepath.ToSlash(relative) + "\n"))
	if err != nil {
		t.Fatal(err)
	}
	want, err := filepath.Abs(relative)
	if err != nil {
		t.Fatal(err)
	}
	if config.DataPath != want {
		t.Fatalf("data path = %q, want working-directory path %q", config.DataPath, want)
	}
}
