package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

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
	owners := make(map[string]string)
	app := Application{
		APIVersion: appAPIVersion,
		Kind:       appKind,
		Metadata:   Metadata{Name: "app1"},
	}

	path := outputPath("c1", "app1")
	relPath := "apps/app1/config-dev.yaml"
	if err := addToProduct(product, owners, path, relPath, app); err != nil {
		t.Fatalf("addToProduct failed: %v", err)
	}
	if len(product[path]) == 0 {
		t.Fatalf("expected rendered bytes in product[%q]", path)
	}
	if owners[path] != relPath {
		t.Errorf("owners[%q] = %q, want %q", path, owners[path], relPath)
	}

	// Verify collision detection: error must mention both claiming configs
	otherRelPath := "apps/other/config-dev.yaml"
	err := addToProduct(product, owners, path, otherRelPath, app)
	if err == nil {
		t.Fatalf("expected collision error in addToProduct, got nil")
	}
	wantErr := fmt.Sprintf("%s: claimed by %q and %q", path, relPath, otherRelPath)
	if err.Error() != wantErr {
		t.Errorf("addToProduct collision error = %q, want %q", err.Error(), wantErr)
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
