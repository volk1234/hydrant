package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCheckCollisions(t *testing.T) {
	tests := map[string]struct {
		intents     []Intent
		wantErr     bool
		errContains string
	}{
		"no collision: distinct apps in same cluster": {
			intents: []Intent{
				{
					Target: Target{Cluster: Cluster{Name: "c1"}},
					Config: ConfigFile{
						RelPath: "apps/app-a/config-dev.yaml",
						Config:  Config{App: App{Name: "app-a"}},
					},
				},
				{
					Target: Target{Cluster: Cluster{Name: "c1"}},
					Config: ConfigFile{
						RelPath: "apps/app-b/config-dev.yaml",
						Config:  Config{App: App{Name: "app-b"}},
					},
				},
			},
			wantErr: false,
		},
		"no collision: same app in distinct clusters": {
			intents: []Intent{
				{
					Target: Target{Cluster: Cluster{Name: "c1"}},
					Config: ConfigFile{
						RelPath: "apps/app-a/config-dev.yaml",
						Config:  Config{App: App{Name: "app-a"}},
					},
				},
				{
					Target: Target{Cluster: Cluster{Name: "c2"}},
					Config: ConfigFile{
						RelPath: "apps/app-a/config-dev.yaml",
						Config:  Config{App: App{Name: "app-a"}},
					},
				},
			},
			wantErr: false,
		},
		"collision: two configs claim same cluster and app name": {
			intents: []Intent{
				{
					Target: Target{Cluster: Cluster{Name: "c1"}},
					Config: ConfigFile{
						RelPath: "apps/first/config-dev.yaml",
						Config:  Config{App: App{Name: "my-service"}},
					},
				},
				{
					Target: Target{Cluster: Cluster{Name: "c1"}},
					Config: ConfigFile{
						RelPath: "apps/second/config-dev.yaml",
						Config:  Config{App: App{Name: "my-service"}},
					},
				},
			},
			wantErr:     true,
			errContains: `c1/my-service.yaml: claimed by "apps/first/config-dev.yaml" and "apps/second/config-dev.yaml"`,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			errs := checkCollisions(tc.intents)
			if tc.wantErr {
				if len(errs) == 0 {
					t.Fatalf("expected error containing %q, got none", tc.errContains)
				}
				found := false
				for _, err := range errs {
					if strings.Contains(err.Error(), tc.errContains) {
						found = true
						break
					}
				}
				if !found {
					t.Fatalf("expected error containing %q, got %v", tc.errContains, errs)
				}
			} else if len(errs) > 0 {
				t.Fatalf("unexpected error: %v", errs)
			}
		})
	}
}

func TestCheckOutDir(t *testing.T) {
	tempDir := t.TempDir()
	repoDir := filepath.Join(tempDir, "repo")
	outSafeDir := filepath.Join(tempDir, "rendered")

	appsDir := "cluster-init/apps"
	clustersDir := "generated/clusters"

	tests := map[string]struct {
		repo        string
		out         string
		wantErr     bool
		errContains string
	}{
		"safe out directory outside repo": {
			repo:    repoDir,
			out:     outSafeDir,
			wantErr: false,
		},
		"out is the repo root itself is rejected": {
			repo:        repoDir,
			out:         repoDir,
			wantErr:     true,
			errContains: "is the checkout itself or above it",
		},
		"out above repo root is rejected": {
			repo:        repoDir,
			out:         tempDir,
			wantErr:     true,
			errContains: "is the checkout itself or above it",
		},
		"out overlaps apps directory": {
			repo:        repoDir,
			out:         filepath.Join(repoDir, appsDir),
			wantErr:     true,
			errContains: "overlaps input directory",
		},
		"out overlaps clusters directory": {
			repo:        repoDir,
			out:         filepath.Join(repoDir, clustersDir),
			wantErr:     true,
			errContains: "overlaps input directory",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			err := checkOutDir(tc.repo, tc.out, appsDir, clustersDir)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tc.errContains)
				}
				if tc.errContains != "" && !strings.Contains(err.Error(), tc.errContains) {
					t.Fatalf("expected error containing %q, got %v", tc.errContains, err)
				}
			} else if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestOutputPath(t *testing.T) {
	got := outputPath("my-cluster", "my-app")
	want := "my-cluster/my-app.yaml"
	if got != want {
		t.Errorf("outputPath = %q, want %q", got, want)
	}
}

func TestAddToProductAndWriteProduct(t *testing.T) {
	product := make(map[string][]byte)
	app := Application{
		APIVersion: appAPIVersion,
		Kind:       appKind,
		Metadata:   Metadata{Name: "app1"},
	}

	path := outputPath("c1", "app1")
	if err := addToProduct(product, path, app); err != nil {
		t.Fatalf("addToProduct failed: %v", err)
	}
	if len(product[path]) == 0 {
		t.Fatalf("expected rendered bytes in product[%q]", path)
	}

	// Write product to temp disk
	outDir := t.TempDir()
	env := "dev"
	if err := writeProduct(outDir, env, product); err != nil {
		t.Fatalf("writeProduct failed: %v", err)
	}

	// Verify target file exists
	expectedFile := filepath.Join(outDir, env, path)
	if _, err := os.Stat(expectedFile); err != nil {
		t.Errorf("expected rendered file %s to exist on disk: %v", expectedFile, err)
	}

	// Verify repeated write cleanly replaces previous tree
	if err := writeProduct(outDir, env, product); err != nil {
		t.Fatalf("repeated writeProduct failed: %v", err)
	}
}
