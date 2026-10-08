package main

import (
	"strings"
	"testing"
)

func TestValidateConfig(t *testing.T) {
	validConfig := func() Config {
		return Config{
			App: App{
				Name: "my-service",
				HelmChart: HelmChart{
					Name:    "my-chart",
					RepoURL: "https://charts.example.com",
					Default: ChartDefaults{
						TargetRevision: "1.0.0",
						ValuesRevision: "1.0.0",
					},
				},
				Destination: Destination{
					Namespace: "my-ns",
				},
			},
		}
	}

	tests := map[string]struct {
		modify      func(c *Config)
		wantErr     bool
		errContains string
	}{
		"valid config with chart name": {
			modify:  func(c *Config) {},
			wantErr: false,
		},
		"valid config with chart path instead of name": {
			modify: func(c *Config) {
				c.App.HelmChart.Name = ""
				c.App.HelmChart.Path = "charts/my-service"
			},
			wantErr: false,
		},
		"both chart name and path specified is rejected": {
			modify: func(c *Config) {
				c.App.HelmChart.Path = "charts/my-service"
			},
			wantErr:     true,
			errContains: "exactly one of app.helmchart.name or app.helmchart.path must be set",
		},
		"neither chart name nor path specified is rejected": {
			modify: func(c *Config) {
				c.App.HelmChart.Name = ""
				c.App.HelmChart.Path = ""
			},
			wantErr:     true,
			errContains: "exactly one of app.helmchart.name or app.helmchart.path must be set",
		},
		"invalid app name with uppercase": {
			modify: func(c *Config) {
				c.App.Name = "MyService"
			},
			wantErr:     true,
			errContains: "app.name must conform to RFC-1123",
		},
		"invalid app name with underscore": {
			modify: func(c *Config) {
				c.App.Name = "my_service"
			},
			wantErr:     true,
			errContains: "app.name must conform to RFC-1123",
		},
		"missing destination namespace": {
			modify: func(c *Config) {
				c.App.Destination.Namespace = ""
			},
			wantErr:     true,
			errContains: "app.destination.namespace is required",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			cfg := validConfig()
			tc.modify(&cfg)
			errs := validateConfig(cfg, "config-dev.yaml")

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

func TestParseFlexBool(t *testing.T) {
	tests := map[string]struct {
		input       any
		wantVal     bool
		wantWarn    bool
		warnContains string
	}{
		"bool true": {
			input:    true,
			wantVal:  true,
			wantWarn: false,
		},
		"bool false": {
			input:    false,
			wantVal:  false,
			wantWarn: false,
		},
		"string true lowercase": {
			input:    "true",
			wantVal:  true,
			wantWarn: false,
		},
		"string true titlecase": {
			input:    "True",
			wantVal:  true,
			wantWarn: false,
		},
		"string false": {
			input:    "false",
			wantVal:  false,
			wantWarn: false,
		},
		"nil treated as false": {
			input:    nil,
			wantVal:  false,
			wantWarn: false,
		},
		"invalid string produces warning": {
			input:        "yes",
			wantVal:      false,
			wantWarn:     true,
			warnContains: "unrecognized value",
		},
		"integer produces warning": {
			input:        1,
			wantVal:      false,
			wantWarn:     true,
			warnContains: "unrecognized value",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			gotVal, warn := parseFlexBool(tc.input, "test.yaml")
			if gotVal != tc.wantVal {
				t.Errorf("parseFlexBool value = %v, want %v", gotVal, tc.wantVal)
			}
			if tc.wantWarn {
				if warn == "" || !strings.Contains(warn, tc.warnContains) {
					t.Errorf("expected warning containing %q, got %q", tc.warnContains, warn)
				}
			} else if warn != "" {
				t.Errorf("unexpected warning: %q", warn)
			}
		})
	}
}

func TestParseConfig(t *testing.T) {
	tests := map[string]struct {
		data        string
		wantErr     bool
		errContains string
		wantName    string
	}{
		"valid config": {
			data: `
app:
  name: test-app
  helmchart:
    name: chart
    repoURL: https://example.com
    default:
      targetRevision: 1.0.0
      valuesRevision: 1.0.0
  destination:
    namespace: test-ns
`,
			wantErr:  false,
			wantName: "test-app",
		},
		"unknown fields rejected": {
			data: `
app:
  name: test-app
unknown_field: true
`,
			wantErr:     true,
			errContains: "field unknown_field not found",
		},
		"malformed yaml": {
			data:        `:::invalid`,
			wantErr:     true,
			errContains: "cannot unmarshal",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			cfg, err := parseConfig([]byte(tc.data), "config.yaml")
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
			if cfg.App.Name != tc.wantName {
				t.Errorf("App.Name = %q, want %q", cfg.App.Name, tc.wantName)
			}
		})
	}
}

func TestArgoNamespaceFor(t *testing.T) {
	tests := map[string]struct {
		appNS       string
		settingsNS  string
		wantNS      string
		wantErr     bool
		errContains string
	}{
		"app namespace takes priority": {
			appNS:      "app-specific-ns",
			settingsNS: "default-settings-ns",
			wantNS:     "app-specific-ns",
			wantErr:    false,
		},
		"falls back to settings namespace": {
			appNS:      "",
			settingsNS: "default-settings-ns",
			wantNS:     "default-settings-ns",
			wantErr:    false,
		},
		"error when both empty": {
			appNS:       "",
			settingsNS:  "",
			wantErr:     true,
			errContains: "argocd.namespace is not set",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			cf := ConfigFile{
				RelPath: "apps/app/config-dev.yaml",
				Config: Config{
					ArgoCD: ArgoCD{Namespace: tc.appNS},
				},
			}
			s := Settings{
				ArgoCD: SettingsArgoCD{Namespace: tc.settingsNS},
			}

			ns, err := argoNamespaceFor(cf, s)
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
			if ns != tc.wantNS {
				t.Errorf("argoNamespaceFor = %q, want %q", ns, tc.wantNS)
			}
		})
	}
}

func TestSecretsEnabled(t *testing.T) {
	cf := ConfigFile{
		RelPath: "apps/app/config.yaml",
		Config: Config{
			ExternalSecretsEnabled: "true",
		},
	}
	enabled, warn := cf.SecretsEnabled()
	if !enabled || warn != "" {
		t.Errorf("SecretsEnabled = %v, warn = %q; want true, empty warn", enabled, warn)
	}
}

