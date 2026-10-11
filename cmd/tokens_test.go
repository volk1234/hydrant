package main

import (
	"strings"
	"testing"
)

func TestClusterTokens(t *testing.T) {
	prefix := "hydrant.sh/"

	tests := map[string]struct {
		cluster     Cluster
		wantTokens  map[string]string
		wantErr     bool
		errContains string
	}{
		"cluster name and valid labels/annotations": {
			cluster: Cluster{
				Name: "cluster-c-1",
				Labels: map[string]string{
					"hydrant.sh/region": "eu-central-1",
					"unrelated-label":   "ignored",
				},
				Annotations: map[string]string{
					"hydrant.sh/account_id": "123456789012",
					"other-annotation":      "ignored",
				},
			},
			wantTokens: map[string]string{
				"[CLUSTER_NAME]": "[CLUSTER_NAME]", // tested separately below
				"[REGION]":       "eu-central-1",
				"[ACCOUNT_ID]":   "123456789012",
			},
			wantErr: false,
		},
		"collision between label and annotation producing same token": {
			cluster: Cluster{
				Name: "cluster-c-1",
				Labels: map[string]string{
					"hydrant.sh/env": "prod",
				},
				Annotations: map[string]string{
					"hydrant.sh/env": "staging",
				},
			},
			wantErr:     true,
			errContains: `token "[ENV]" is already defined`,
		},
		"collision between case-insensitive label keys": {
			cluster: Cluster{
				Name: "cluster-c-1",
				Labels: map[string]string{
					"hydrant.sh/tier": "frontend",
					"hydrant.sh/TIER": "backend",
				},
			},
			wantErr:     true,
			errContains: `token "[TIER]" is already defined`,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			tokens, err := clusterTokens(tc.cluster, prefix)
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
			if tokens["[CLUSTER_NAME]"] != tc.cluster.Name {
				t.Errorf("[CLUSTER_NAME] = %q, want %q", tokens["[CLUSTER_NAME]"], tc.cluster.Name)
			}
			for k, wantVal := range tc.wantTokens {
				if k == "[CLUSTER_NAME]" {
					continue
				}
				if gotVal, ok := tokens[k]; !ok || gotVal != wantVal {
					t.Errorf("token %s = %q, want %q", k, gotVal, wantVal)
				}
			}
		})
	}
}

func TestSubstituteTokens(t *testing.T) {
	prefix := "hydrant.sh/"
	cluster := Cluster{
		Name: "prod-eu",
		Labels: map[string]string{
			"hydrant.sh/region": "eu-west-1",
			"hydrant.sh/stage":  "production",
		},
	}

	tests := map[string]struct {
		input       string
		wantOutput  string
		wantErr     bool
		errContains string
	}{
		"successful substitution": {
			input:      "cluster: [CLUSTER_NAME]\nregion: [REGION]\nenv: [STAGE]\n",
			wantOutput: "cluster: prod-eu\nregion: eu-west-1\nenv: production\n",
			wantErr:    false,
		},
		"repeated token is substituted in all positions": {
			input:      "host: [CLUSTER_NAME].[REGION].internal",
			wantOutput: "host: prod-eu.eu-west-1.internal",
			wantErr:    false,
		},
		"missing token fails with error": {
			input:       "db_url: postgres://[DB_HOST]:5432",
			wantErr:     true,
			errContains: `no value for token "[DB_HOST]"`,
		},
		"empty token value in cluster treated as missing": {
			input:      "cluster: [CLUSTER_NAME]\nval: [STAGE]",
			wantErr:    false,
			wantOutput: "cluster: prod-eu\nval: production",
		},
		"no tokens in string passes through untouched": {
			input:      "key: value\ncount: 42\n",
			wantOutput: "key: value\ncount: 42\n",
			wantErr:    false,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			output, err := substituteTokens(tc.input, cluster, prefix)
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
			if output != tc.wantOutput {
				t.Errorf("substituteTokens mismatch\n--- got ---\n%s\n--- want ---\n%s", output, tc.wantOutput)
			}
		})
	}
}
