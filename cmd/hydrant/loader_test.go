package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadOneConfig(t *testing.T) {
	tempRepo := t.TempDir()
	appDir := filepath.Join(tempRepo, "apps", "my-service")
	if err := os.MkdirAll(appDir, 0o755); err != nil {
		t.Fatal(err)
	}

	validConfig := `
app:
  name: my-service
  helmchart:
    name: chart
    repoURL: https://example.com
    default:
      targetRevision: 1.0.0
      valuesRevision: 1.0.0
  destination:
    namespace: my-ns
`
	validConfigPath := filepath.Join(appDir, "config-dev.yaml")
	if err := os.WriteFile(validConfigPath, []byte(validConfig), 0o644); err != nil {
		t.Fatal(err)
	}

	// 1. Success case: directory matches app.name
	cf, errs := loadOneConfig(tempRepo, validConfigPath)
	if len(errs) > 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if cf.Config.App.Name != "my-service" {
		t.Errorf("App.Name = %q, want my-service", cf.Config.App.Name)
	}
	if cf.Dir != "apps/my-service" {
		t.Errorf("Dir = %q, want apps/my-service", cf.Dir)
	}

	// 2. Failure case: app.name does not match directory
	mismatchedConfig := strings.Replace(validConfig, "name: my-service", "name: other-service", 1)
	mismatchedPath := filepath.Join(appDir, "config-live.yaml")
	if err := os.WriteFile(mismatchedPath, []byte(mismatchedConfig), 0o644); err != nil {
		t.Fatal(err)
	}

	_, errs = loadOneConfig(tempRepo, mismatchedPath)
	if len(errs) == 0 {
		t.Fatalf("expected error for mismatched directory name, got none")
	}
	found := false
	for _, err := range errs {
		if strings.Contains(err.Error(), "does not match directory") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected 'does not match directory' error, got %v", errs)
	}

	// 3. Failure case: non-existent file
	_, errs = loadOneConfig(tempRepo, filepath.Join(appDir, "missing.yaml"))
	if len(errs) == 0 {
		t.Fatalf("expected error for missing file, got none")
	}
}

func TestLoadConfigsAndFindStray(t *testing.T) {
	tempRepo := t.TempDir()
	appsDir := "apps"
	app1Dir := filepath.Join(tempRepo, appsDir, "svc1")
	app2Dir := filepath.Join(tempRepo, appsDir, "svc2")

	for _, d := range []string{app1Dir, app2Dir} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}

	svc1Dev := `
app:
  name: svc1
  helmchart:
    name: chart
    repoURL: https://example.com
    default:
      targetRevision: 1.0.0
      valuesRevision: 1.0.0
  destination:
    namespace: ns1
`
	svc2Dev := strings.ReplaceAll(svc1Dev, "svc1", "svc2")
	svc2Stray := svc2Dev // config-unknown.yaml

	if err := os.WriteFile(filepath.Join(app1Dir, "config-dev.yaml"), []byte(svc1Dev), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(app2Dir, "config-dev.yaml"), []byte(svc2Dev), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(app2Dir, "config-unknown.yaml"), []byte(svc2Stray), 0o644); err != nil {
		t.Fatal(err)
	}

	// loadConfigs for dev
	configs, cfgErrs, err := loadConfigs(tempRepo, "dev", appsDir)
	if err != nil {
		t.Fatalf("loadConfigs unexpected error: %v", err)
	}
	if len(cfgErrs) > 0 {
		t.Fatalf("loadConfigs unexpected cfgErrs: %v", cfgErrs)
	}
	if len(configs) != 2 {
		t.Fatalf("expected 2 configs, got %d", len(configs))
	}

	// findStrayConfigs: only "dev" and "live" are known envs; "unknown" should be stray
	knownEnvs := []string{"dev", "live"}
	strays, err := findStrayConfigs(tempRepo, knownEnvs, appsDir)
	if err != nil {
		t.Fatalf("findStrayConfigs unexpected error: %v", err)
	}
	if len(strays) != 1 || !strings.Contains(strays[0], "environment \"unknown\" has no cluster directory") {
		t.Fatalf("unexpected strays: %v", strays)
	}
}

func TestLoadConfigs_DirectoryNotFound(t *testing.T) {
	tempRepo := t.TempDir()
	_, _, err := loadConfigs(tempRepo, "dev", "deploy/nope")
	if err == nil {
		t.Fatal("expected error for non-existent apps directory, got nil")
	}
	want := "deploy/nope: directory not found"
	if err.Error() != want {
		t.Errorf("error = %q, want %q", err.Error(), want)
	}
}

