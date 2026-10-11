package main

import (
	"strings"
	"testing"
)

func TestResolveTargets(t *testing.T) {
	profileLabel := "hydrant.sh/profile-"

	clusters := map[string]Cluster{
		"c-prod-1": {
			Name: "c-prod-1",
			Labels: map[string]string{
				"hydrant.sh/profile-backend": "true",
			},
		},
		"c-prod-2": {
			Name: "c-prod-2",
			Labels: map[string]string{
				"hydrant.sh/profile-backend": "true",
			},
		},
		"c-stage-1": {
			Name: "c-stage-1",
			Labels: map[string]string{
				"hydrant.sh/profile-frontend": "true",
			},
		},
	}

	tests := map[string]struct {
		cfg          ConfigFile
		wantTargets  []string
		wantErr      bool
		errContains  string
		wantWarn     bool
		warnContains string
	}{
		"match clusters by profile": {
			cfg: ConfigFile{
				RelPath: "apps/api/config-dev.yaml",
				Config: Config{
					App: App{
						Name: "api",
						Destination: Destination{
							Profile: "backend",
						},
					},
				},
			},
			wantTargets: []string{"c-prod-1", "c-prod-2"},
			wantErr:     false,
		},
		"explicit cluster override adds cluster": {
			cfg: ConfigFile{
				RelPath: "apps/api/config-dev.yaml",
				Config: Config{
					App: App{
						Name: "api",
						Destination: Destination{
							Clusters: map[string]ClusterOverride{
								"c-stage-1": {Namespace: "custom-ns"},
							},
						},
					},
				},
			},
			wantTargets: []string{"c-stage-1"},
			wantErr:     false,
		},
		"override targeting non-existent cluster produces error": {
			cfg: ConfigFile{
				RelPath: "apps/api/config-dev.yaml",
				Config: Config{
					App: App{
						Name: "api",
						Destination: Destination{
							Clusters: map[string]ClusterOverride{
								"non-existent-cluster": {},
							},
						},
					},
				},
			},
			wantErr:     true,
			errContains: "cluster \"non-existent-cluster\" not found in this environment",
		},
		"profile matching nothing produces warning": {
			cfg: ConfigFile{
				RelPath: "apps/api/config-dev.yaml",
				Config: Config{
					App: App{
						Name: "api",
						Destination: Destination{
							Profile: "database",
						},
					},
				},
			},
			wantWarn:     true,
			warnContains: "profile \"database\" matched nothing",
		},
		"deterministic sorting of target clusters": {
			cfg: ConfigFile{
				RelPath: "apps/api/config-dev.yaml",
				Config: Config{
					App: App{
						Name: "api",
						Destination: Destination{
							Profile: "backend",
							Clusters: map[string]ClusterOverride{
								"c-stage-1": {},
							},
						},
					},
				},
			},
			wantTargets: []string{"c-prod-1", "c-prod-2", "c-stage-1"},
			wantErr:     false,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			targets, errs, warns := resolveTargets(tc.cfg, clusters, profileLabel)

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

			if tc.wantWarn {
				found := false
				for _, warn := range warns {
					if strings.Contains(warn, tc.warnContains) {
						found = true
						break
					}
				}
				if !found {
					t.Fatalf("expected warning containing %q, got %v", tc.warnContains, warns)
				}
			}

			if tc.wantTargets != nil {
				if len(targets) != len(tc.wantTargets) {
					t.Fatalf("target count = %d, want %d", len(targets), len(tc.wantTargets))
				}
				for i, target := range targets {
					if target.Cluster.Name != tc.wantTargets[i] {
						t.Errorf("target[%d] = %q, want %q", i, target.Cluster.Name, tc.wantTargets[i])
					}
				}
			}
		})
	}
}

func TestFilterLegacy(t *testing.T) {
	intents := []Intent{
		{
			Config: ConfigFile{
				RelPath: "apps/legacy-app/config-dev.yaml",
				Config: Config{
					LegacyEnabled: "true",
				},
			},
		},
		{
			Config: ConfigFile{
				RelPath: "apps/active-app/config-dev.yaml",
				Config: Config{
					LegacyEnabled: "false",
				},
			},
		},
		{
			Config: ConfigFile{
				RelPath: "apps/normal-app/config-dev.yaml",
				Config:  Config{},
			},
		},
	}

	filtered, skipped := filterLegacy(intents)
	if skipped != 1 {
		t.Errorf("skipped legacy count = %d, want 1", skipped)
	}
	if len(filtered) != 2 {
		t.Fatalf("filtered count = %d, want 2", len(filtered))
	}
	for _, it := range filtered {
		if it.Config.Config.LegacyEnabled == "true" {
			t.Errorf("legacy app %s was not filtered out", it.Config.RelPath)
		}
	}
}

func TestFilterIntents(t *testing.T) {
	intents := []Intent{
		{Target: Target{Cluster: Cluster{Name: "c1"}}},
		{Target: Target{Cluster: Cluster{Name: "c2"}}},
		{Target: Target{Cluster: Cluster{Name: "c3"}}},
	}
	render := map[string]Cluster{
		"c1": {Name: "c1"},
		"c3": {Name: "c3"},
	}

	result := filterIntents(intents, render)
	if len(result) != 2 {
		t.Fatalf("filterIntents count = %d, want 2", len(result))
	}
	for _, it := range result {
		if it.Target.Cluster.Name == "c2" {
			t.Errorf("cluster c2 should have been filtered out")
		}
	}
}
