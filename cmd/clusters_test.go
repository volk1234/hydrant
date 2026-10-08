package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseClusterSecret(t *testing.T) {
	tests := map[string]struct {
		data        string
		wantErr     bool
		errContains string
		wantName    string
	}{
		"valid cluster secret": {
			data: `
kind: Secret
metadata:
  labels:
    argocd.argoproj.io/secret-type: cluster
    hydrant.sh/region: eu-west-1
  annotations:
    hydrant.sh/account: "123"
stringData:
  name: prod-eu-1
`,
			wantErr:  false,
			wantName: "prod-eu-1",
		},
		"wrong kind": {
			data: `
kind: ConfigMap
metadata:
  labels:
    argocd.argoproj.io/secret-type: cluster
stringData:
  name: prod-eu-1
`,
			wantErr:     true,
			errContains: "expected kind: Secret",
		},
		"missing secret-type label": {
			data: `
kind: Secret
metadata:
  labels:
    other: value
stringData:
  name: prod-eu-1
`,
			wantErr:     true,
			errContains: "expected label argocd.argoproj.io/secret-type=cluster",
		},
		"empty stringData name": {
			data: `
kind: Secret
metadata:
  labels:
    argocd.argoproj.io/secret-type: cluster
stringData:
  name: ""
`,
			wantErr:     true,
			errContains: "stringData.name is empty or missing",
		},
		"invalid RFC-1123 name": {
			data: `
kind: Secret
metadata:
  labels:
    argocd.argoproj.io/secret-type: cluster
stringData:
  name: Prod_Cluster_1
`,
			wantErr:     true,
			errContains: "stringData.name must conform to RFC-1123",
		},
		"malformed yaml": {
			data:        `:::not valid yaml`,
			wantErr:     true,
			errContains: "cannot unmarshal",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			cluster, err := parseClusterSecret([]byte(tc.data), "cluster.yaml")
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tc.errContains)
				}
				if tc.errContains != "" && !strings.Contains(err.Error(), tc.errContains) {
					t.Fatalf("expected error containing %q, got %v", tc.errContains, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if cluster.Name != tc.wantName {
				t.Errorf("Cluster.Name = %q, want %q", cluster.Name, tc.wantName)
			}
		})
	}
}

func TestFilterByPartition(t *testing.T) {
	clusters := map[string]Cluster{
		"c1": {Name: "c1", Labels: map[string]string{"partition": "true"}},
		"c2": {Name: "c2", Labels: map[string]string{"partition": "false"}},
		"c3": {Name: "c3", Labels: map[string]string{}},
	}

	tests := map[string]struct {
		label string
		want  []string
	}{
		"empty label returns all clusters": {
			label: "",
			want:  []string{"c1", "c2", "c3"},
		},
		"partition label filters to matching clusters only": {
			label: "partition",
			want:  []string{"c1"},
		},
		"unmatched label returns empty map": {
			label: "non-existent",
			want:  []string{},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			filtered := filterByPartition(clusters, tc.label)
			if len(filtered) != len(tc.want) {
				t.Fatalf("filtered count = %d, want %d", len(filtered), len(tc.want))
			}
			for _, name := range tc.want {
				if _, ok := filtered[name]; !ok {
					t.Errorf("expected cluster %q in result", name)
				}
			}
		})
	}
}

func TestListEnvsAndLoadClusters(t *testing.T) {
	tempRepo := t.TempDir()
	clustersDir := "clusters"
	clustersPath := filepath.Join(tempRepo, clustersDir)

	if err := os.MkdirAll(filepath.Join(clustersPath, "dev"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(clustersPath, "live"), 0o755); err != nil {
		t.Fatal(err)
	}

	// listEnvs
	envs, err := listEnvs(tempRepo, clustersDir)
	if err != nil {
		t.Fatalf("listEnvs unexpected error: %v", err)
	}
	if len(envs) != 2 {
		t.Fatalf("expected 2 envs, got %d (%v)", len(envs), envs)
	}

	// loadClusters with no files in dev
	_, err = loadClusters(tempRepo, "dev", clustersDir)
	if err == nil || !strings.Contains(err.Error(), "no cluster secrets found") {
		t.Fatalf("expected 'no cluster secrets found', got %v", err)
	}

	// Add valid cluster secret to dev
	validSecret := `
kind: Secret
metadata:
  labels:
    argocd.argoproj.io/secret-type: cluster
stringData:
  name: cluster-1
`
	if err := os.WriteFile(filepath.Join(clustersPath, "dev", "c1.yaml"), []byte(validSecret), 0o644); err != nil {
		t.Fatal(err)
	}

	clusters, err := loadClusters(tempRepo, "dev", clustersDir)
	if err != nil {
		t.Fatalf("loadClusters unexpected error: %v", err)
	}
	if len(clusters) != 1 || clusters["cluster-1"].Name != "cluster-1" {
		t.Fatalf("unexpected loaded clusters: %+v", clusters)
	}

	// Add duplicate cluster name
	if err := os.WriteFile(filepath.Join(clustersPath, "dev", "c2.yaml"), []byte(validSecret), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err = loadClusters(tempRepo, "dev", clustersDir)
	if err == nil || !strings.Contains(err.Error(), "duplicate cluster name") {
		t.Fatalf("expected 'duplicate cluster name', got %v", err)
	}

	// Non-existent env
	_, err = loadClusters(tempRepo, "staging", clustersDir)
	if err == nil || !strings.Contains(err.Error(), "environment directory not found") {
		t.Fatalf("expected 'environment directory not found', got %v", err)
	}
}
