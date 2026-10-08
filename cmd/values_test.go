package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.yaml.in/yaml/v3"
)

func parseValuesHelper(t *testing.T, raw string) *yaml.Node {
	t.Helper()
	node, err := parseValues([]byte(raw), "test.yaml")
	if err != nil {
		t.Fatalf("parseValues unexpected error: %v", err)
	}
	return node
}

func marshalNodeHelper(t *testing.T, node *yaml.Node) string {
	t.Helper()
	data, err := yaml.Marshal(node)
	if err != nil {
		t.Fatalf("yaml.Marshal unexpected error: %v", err)
	}
	return strings.TrimSpace(string(data))
}

func TestCheckValuesNode_Yaml11Bools(t *testing.T) {
	tests := map[string]struct {
		input       string
		wantErr     bool
		errContains string
	}{
		"unquoted on": {
			input:       "on: true\n",
			wantErr:     true,
			errContains: "is read by Helm as a boolean; quote it",
		},
		"unquoted off": {
			input:       "off: false\n",
			wantErr:     true,
			errContains: "is read by Helm as a boolean; quote it",
		},
		"unquoted yes": {
			input:       "yes: 123\n",
			wantErr:     true,
			errContains: "is read by Helm as a boolean; quote it",
		},
		"unquoted no": {
			input:       "no: 456\n",
			wantErr:     true,
			errContains: "is read by Helm as a boolean; quote it",
		},
		"unquoted y": {
			input:       "y: true\n",
			wantErr:     true,
			errContains: "is read by Helm as a boolean; quote it",
		},
		"unquoted n": {
			input:       "n: false\n",
			wantErr:     true,
			errContains: "is read by Helm as a boolean; quote it",
		},
		"quoted 'on'": {
			input:   "'on': true\n",
			wantErr: false,
		},
		"quoted \"no\"": {
			input:   "\"no\": false\n",
			wantErr: false,
		},
		"quoted \"yes\"": {
			input:   "\"yes\": \"value\"\n",
			wantErr: false,
		},
		"nested unquoted bool key": {
			input:       "server:\n  settings:\n    off: true\n",
			wantErr:     true,
			errContains: "is read by Helm as a boolean; quote it",
		},
		"valid normal key": {
			input:   "enabled: true\nreplicaCount: 2\n",
			wantErr: false,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := parseValues([]byte(tc.input), "test.yaml")
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

func TestCheckValuesNode_DuplicateKeys(t *testing.T) {
	tests := map[string]struct {
		input       string
		wantErr     bool
		errContains string
	}{
		"duplicate top-level keys": {
			input:       "replicaCount: 1\nreplicaCount: 2\n",
			wantErr:     true,
			errContains: "duplicate key \"replicaCount\"",
		},
		"duplicate nested keys": {
			input:       "ingress:\n  host: a.example.com\n  host: b.example.com\n",
			wantErr:     true,
			errContains: "duplicate key \"host\"",
		},
		"same key in different sub-maps is allowed": {
			input:   "serviceA:\n  port: 80\nserviceB:\n  port: 80\n",
			wantErr: false,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := parseValues([]byte(tc.input), "test.yaml")
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

func TestMergeValues(t *testing.T) {
	tests := map[string]struct {
		lowerYAML string
		upperYAML string
		expected  string
	}{
		"deep merge mappings": {
			lowerYAML: "server:\n  host: 127.0.0.1\n  port: 80\n",
			upperYAML: "server:\n  port: 8080\n  timeout: 30\n",
			expected:  "server:\n    host: 127.0.0.1\n    port: 8080\n    timeout: 30",
		},
		"list replacement (lists are overwritten, not merged)": {
			lowerYAML: "args:\n  - --debug\n  - --verbose\n",
			upperYAML: "args:\n  - --quiet\n",
			expected:  "args:\n    - --quiet",
		},
		"scalar overrides map": {
			lowerYAML: "data:\n  nested: value\n",
			upperYAML: "data: scalar\n",
			expected:  "data: scalar",
		},
		"map overrides scalar": {
			lowerYAML: "data: scalar\n",
			upperYAML: "data:\n  nested: value\n",
			expected:  "data:\n    nested: value",
		},
		"lower is nil": {
			lowerYAML: "",
			upperYAML: "key: value\n",
			expected:  "key: value",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			var lower *yaml.Node
			if tc.lowerYAML != "" {
				lower = parseValuesHelper(t, tc.lowerYAML)
			}
			upper := parseValuesHelper(t, tc.upperYAML)

			merged := mergeValues(lower, upper)
			normalizeValues(merged)

			got := marshalNodeHelper(t, merged)
			if got != tc.expected {
				t.Errorf("mergeValues mismatch\n--- got ---\n%s\n--- expected ---\n%s", got, tc.expected)
			}
		})
	}
}

func TestNormalizeValues_Determinism(t *testing.T) {
	tests := map[string]struct {
		input    string
		expected string
	}{
		"keys are sorted alphabetically": {
			input:    "zebra: 1\napple: 2\nbanana: 3\n",
			expected: "apple: 2\nbanana: 3\nzebra: 1",
		},
		"nested keys are sorted alphabetically": {
			input:    "root:\n  z: 1\n  a: 2\n  b:\n    beta: 1\n    alpha: 2\n",
			expected: "root:\n    a: 2\n    b:\n        alpha: 2\n        beta: 1\n    z: 1",
		},
		"comments are stripped": {
			input:    "# Header comment\nfoo: bar # Line comment\n# Footer comment\n",
			expected: "foo: bar",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			node := parseValuesHelper(t, tc.input)
			normalizeValues(node)
			got := marshalNodeHelper(t, node)
			if got != tc.expected {
				t.Errorf("normalizeValues mismatch\n--- got ---\n%s\n--- expected ---\n%s", got, tc.expected)
			}
		})
	}
}

func TestBuildValuesAndLayers(t *testing.T) {
	tempRepo := t.TempDir()
	appRelDir := "apps/demo"
	appDir := filepath.Join(tempRepo, appRelDir)
	if err := os.MkdirAll(appDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Layer 1: values.yaml
	if err := os.WriteFile(filepath.Join(appDir, "values.yaml"), []byte("replicaCount: 1\nenv: base\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Layer 2: values.dev.yaml
	if err := os.WriteFile(filepath.Join(appDir, "values.dev.yaml"), []byte("env: dev\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Layer 3: values.c1.yaml
	if err := os.WriteFile(filepath.Join(appDir, "values.c1.yaml"), []byte("cluster: c1\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Test readValuesLayers directly
	layers, err := readValuesLayers(tempRepo, appRelDir, "dev", "c1")
	if err != nil {
		t.Fatalf("readValuesLayers failed: %v", err)
	}
	if len(layers) != 3 {
		t.Fatalf("expected 3 layers, got %d", len(layers))
	}

	// Test buildValues with inline_values tokens
	cfg := ConfigFile{
		Dir:     appRelDir,
		RelPath: appRelDir + "/config-dev.yaml",
		Config: Config{
			InlineValues: "inlineCluster: [CLUSTER_NAME]\nregion: [REGION]\n",
		},
	}
	target := Target{
		Cluster: Cluster{
			Name: "c1",
			Labels: map[string]string{
				"hydrant.sh/region": "eu-west-1",
			},
		},
	}

	vo, err := buildValues(tempRepo, cfg, target, "dev", "hydrant.sh/")
	if err != nil {
		t.Fatalf("buildValues failed: %v", err)
	}
	if vo.IsZero() {
		t.Fatal("expected non-zero ValuesObject")
	}

	marshaled := marshalNodeHelper(t, vo.node)
	expectedSubstrings := []string{
		"cluster: c1",
		"env: dev",
		"inlineCluster: c1",
		"region: eu-west-1",
		"replicaCount: 1",
	}
	for _, substr := range expectedSubstrings {
		if !strings.Contains(marshaled, substr) {
			t.Errorf("expected marshaled output to contain %q, got:\n%s", substr, marshaled)
		}
	}
}

func TestValuesObject(t *testing.T) {
	// Zero value
	var zero ValuesObject
	if !zero.IsZero() {
		t.Errorf("zero ValuesObject should be IsZero=true")
	}

	// Non-zero value
	node := parseValuesHelper(t, "key: value\n")
	vo := ValuesObject{node: node}
	if vo.IsZero() {
		t.Errorf("populated ValuesObject should be IsZero=false")
	}

	marshaled, err := vo.MarshalYAML()
	if err != nil || marshaled != node {
		t.Errorf("MarshalYAML mismatch: %v, %v", marshaled, err)
	}
}

