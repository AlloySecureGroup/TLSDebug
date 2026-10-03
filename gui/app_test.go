package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestShellQuote(t *testing.T) {
	actual := shellQuote("/tmp/it's here/ca.crt")
	expected := "'/tmp/it'\"'\"'s here/ca.crt'"
	if actual != expected {
		t.Fatalf("shellQuote() = %q, want %q", actual, expected)
	}
}

func TestResolveProxyBinaryUsesConfiguredPath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "custom-proxy")
	if err := os.WriteFile(path, []byte("proxy"), 0700); err != nil {
		t.Fatal(err)
	}

	app := NewApp()
	resolved, err := app.resolveProxyBinary(Settings{ProxyBinary: path})
	if err != nil {
		t.Fatal(err)
	}
	if resolved != path {
		t.Fatalf("resolveProxyBinary() = %q, want %q", resolved, path)
	}
}

func TestUpdateSettingsRejectsInvalidPorts(t *testing.T) {
	app := NewApp()
	_, err := app.UpdateSettings(Settings{
		ProxyPort:   0,
		MonitorPort: 4040,
		DataDir:     t.TempDir(),
	})
	if err == nil {
		t.Fatal("UpdateSettings() accepted an invalid proxy port")
	}
}
