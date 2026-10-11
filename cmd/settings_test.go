package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCheckRepoPath(t *testing.T) {
	tests := map[string]struct {
		input       string
		wantErr     bool
		errContains string
	}{
		"valid relative path": {
			input:   "cluster-init/apps",
			wantErr: false,
		},
		"single directory name": {
			input:   "apps",
			wantErr: false,
		},
		"leading slash forbidden": {
			input:       "/cluster-init/apps",
			wantErr:     true,
			errContains: "must not start with /",
		},
		"backslash forbidden": {
			input:       "cluster-init\\apps",
			wantErr:     true,
			errContains: "use / as separator, not \\",
		},
		"parent traversal forbidden": {
			input:       "apps/../clusters",
			wantErr:     true,
			errContains: `must not contain ".."`,
		},
		"trailing slash not in normal form": {
			input:       "apps/",
			wantErr:     true,
			errContains: "not in normal form",
		},
		"double slash not in normal form": {
			input:       "cluster-init//apps",
			wantErr:     true,
			errContains: "not in normal form",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			errs := checkRepoPath("testKey", tc.input)
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
				t.Fatalf("unexpected errors: %v", errs)
			}
		})
	}
}

func TestInsideRepoPath(t *testing.T) {
	tests := map[string]struct {
		a    string
		b    string
		want bool
	}{
		"identical paths": {
			a:    "apps",
			b:    "apps",
			want: true,
		},
		"a is nested inside b": {
			a:    "cluster-init/apps",
			b:    "cluster-init",
			want: true,
		},
		"a is sibling with common prefix": {
			a:    "cluster-init-apps",
			b:    "cluster-init",
			want: false,
		},
		"completely distinct paths": {
			a:    "cluster-init/apps",
			b:    "generated/clusters",
			want: false,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			got := insideRepoPath(tc.a, tc.b)
			if got != tc.want {
				t.Errorf("insideRepoPath(%q, %q) = %v, want %v", tc.a, tc.b, got, tc.want)
			}
		})
	}
}

func TestCheckLabel(t *testing.T) {
	tests := map[string]struct {
		key         string
		val         string
		wantErr     bool
		errContains string
	}{
		"valid standard label": {
			key:     "app.kubernetes.io/name",
			val:     "hydrant",
			wantErr: false,
		},
		"valid AWS ALB readiness gate label": {
			key:     "elbv2.k8s.aws/pod-readiness-gate-inject",
			val:     "enabled",
			wantErr: false,
		},
		"valid label without prefix": {
			key:     "environment",
			val:     "production",
			wantErr: false,
		},
		"empty value is allowed in k8s": {
			key:     "isolated",
			val:     "",
			wantErr: false,
		},
		"invalid uppercase in DNS subdomain prefix": {
			key:         "MyDomain.com/app",
			val:         "test",
			wantErr:     true,
			errContains: "is not a valid DNS subdomain",
		},
		"invalid underscore in DNS subdomain prefix": {
			key:         "my_domain.com/app",
			val:         "test",
			wantErr:     true,
			errContains: "is not a valid DNS subdomain",
		},
		"prefix exceeds 253 characters": {
			key:         strings.Repeat("a", 254) + ".com/app",
			val:         "test",
			wantErr:     true,
			errContains: "is not a valid DNS subdomain",
		},
		"name exceeds 63 characters": {
			key:         "example.com/" + strings.Repeat("a", 64),
			val:         "test",
			wantErr:     true,
			errContains: "is not a valid label name",
		},
		"name with invalid character": {
			key:         "app#name",
			val:         "test",
			wantErr:     true,
			errContains: "is not a valid label name",
		},
		"value exceeds 63 characters": {
			key:         "app",
			val:         strings.Repeat("v", 64),
			wantErr:     true,
			errContains: "is not a valid label value",
		},
		"value with invalid character": {
			key:         "app",
			val:         "test@val",
			wantErr:     true,
			errContains: "is not a valid label value",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			err := checkLabel(tc.key, tc.val)
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

func TestValidateSettings(t *testing.T) {
	validSettings := func() Settings {
		return Settings{
			LabelPrefix:    "hydrant.sh/",
			SecretsRepoURL: "git@github.com:org/repo.git",
			Apps:           "cluster-init/apps",
			Clusters:       "generated/clusters",
			ArgoCD: SettingsArgoCD{
				Namespace: "argocd",
			},
		}
	}

	tests := map[string]struct {
		modify      func(s *Settings)
		wantErr     bool
		errContains string
	}{
		"valid settings passes": {
			modify:  func(s *Settings) {},
			wantErr: false,
		},
		"missing labelPrefix": {
			modify:      func(s *Settings) { s.LabelPrefix = "" },
			wantErr:     true,
			errContains: "labelPrefix is required",
		},
		"labelPrefix must end with slash": {
			modify:      func(s *Settings) { s.LabelPrefix = "hydrant.sh" },
			wantErr:     true,
			errContains: "must end with /",
		},
		"missing secretsRepoURL": {
			modify:      func(s *Settings) { s.SecretsRepoURL = "" },
			wantErr:     true,
			errContains: "secretsRepoURL is required",
		},
		"missing apps": {
			modify:      func(s *Settings) { s.Apps = "" },
			wantErr:     true,
			errContains: "apps is required",
		},
		"missing clusters": {
			modify:      func(s *Settings) { s.Clusters = "" },
			wantErr:     true,
			errContains: "clusters is required",
		},
		"apps and clusters cannot be the same directory": {
			modify: func(s *Settings) {
				s.Apps = "shared/dir"
				s.Clusters = "shared/dir"
			},
			wantErr:     true,
			errContains: "must not be the same directory or nested",
		},
		"apps cannot be nested inside clusters": {
			modify: func(s *Settings) {
				s.Clusters = "infra"
				s.Apps = "infra/apps"
			},
			wantErr:     true,
			errContains: "must not be the same directory or nested",
		},
		"invalid argocd namespace name": {
			modify: func(s *Settings) {
				s.ArgoCD.Namespace = "INVALID_UPPERCASE"
			},
			wantErr:     true,
			errContains: "not a valid namespace name",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			s := validSettings()
			tc.modify(&s)
			errs := validateSettings(s)
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

func TestCheckPrefixInClusters(t *testing.T) {
	prefix := "hydrant.sh/"

	tests := map[string]struct {
		clusters    map[string]Cluster
		wantErr     bool
		errContains string
	}{
		"prefix matches in cluster labels": {
			clusters: map[string]Cluster{
				"c1": {
					Labels: map[string]string{
						"hydrant.sh/region": "eu-central-1",
					},
				},
			},
			wantErr: false,
		},
		"prefix matches in cluster annotations": {
			clusters: map[string]Cluster{
				"c1": {
					Annotations: map[string]string{
						"hydrant.sh/account": "123",
					},
				},
			},
			wantErr: false,
		},
		"no cluster has prefix in labels or annotations": {
			clusters: map[string]Cluster{
				"c1": {
					Labels: map[string]string{
						"other.io/region": "us-east-1",
					},
					Annotations: map[string]string{
						"unrelated": "true",
					},
				},
			},
			wantErr:     true,
			errContains: "no cluster in this environment has labels or annotations with this prefix",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			err := checkPrefixInClusters(prefix, tc.clusters)
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

func TestParseAndLoadSettings(t *testing.T) {
	validYAML := `
labelPrefix: hydrant.sh/
secretsRepoURL: git@github.com:example/secrets.git
apps: cluster-init/apps
clusters: generated/clusters
argocd:
  namespace: argocd
`
	// parseSettings
	s, err := parseSettings([]byte(validYAML))
	if err != nil {
		t.Fatalf("parseSettings failed: %v", err)
	}
	if s.LabelPrefix != "hydrant.sh/" {
		t.Errorf("LabelPrefix = %q, want hydrant.sh/", s.LabelPrefix)
	}

	// parseSettings unknown fields rejected
	_, err = parseSettings([]byte(validYAML + "\nextraField: true\n"))
	if err == nil || !strings.Contains(err.Error(), "field extraField not found") {
		t.Errorf("expected error for extra field, got %v", err)
	}

	// loadSettings
	tempRepo := t.TempDir()
	if err := os.WriteFile(filepath.Join(tempRepo, "hydrant.yaml"), []byte(validYAML), 0o644); err != nil {
		t.Fatal(err)
	}

	loaded, errs := loadSettings(tempRepo)
	if len(errs) > 0 {
		t.Fatalf("loadSettings failed: %v", errs)
	}
	if loaded.Apps != "cluster-init/apps" {
		t.Errorf("Apps = %q, want cluster-init/apps", loaded.Apps)
	}

	// loadSettings missing file
	_, errs = loadSettings(filepath.Join(tempRepo, "nonexistent"))
	if len(errs) == 0 {
		t.Errorf("expected error for missing hydrant.yaml, got none")
	}
}

func TestProfileLabel(t *testing.T) {
	s := Settings{LabelPrefix: "custom.prefix/"}
	got := s.ProfileLabel()
	want := "custom.prefix/profile-"
	if got != want {
		t.Errorf("ProfileLabel() = %q, want %q", got, want)
	}
}
