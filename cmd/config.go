package main

import (
	"bytes"
	"fmt"
	"strings"

	"go.yaml.in/yaml/v3"
)

type Config struct {
	App                    App    `yaml:"app"`
	ArgoCD                 ArgoCD `yaml:"argocd"`
	InlineValues           string `yaml:"inline_values"`
	ExternalSecretsEnabled any    `yaml:"externalSecretsEnabled"`
	LegacyEnabled          string `yaml:"legacyEnabled"`
}

type App struct {
	Name        string      `yaml:"name"`
	HelmChart   HelmChart   `yaml:"helmchart"`
	Destination Destination `yaml:"destination"`
}

type HelmChart struct {
	Name        string        `yaml:"name"`
	Path        string        `yaml:"path"`
	RepoURL     string        `yaml:"repoURL"`
	ReleaseName string        `yaml:"release_name"`
	Default     ChartDefaults `yaml:"default"`
	Upgrade     struct {
		Auto bool `yaml:"auto"`
	} `yaml:"upgrade"`
}

type ChartDefaults struct {
	TargetRevision  string `yaml:"targetRevision"`
	ValuesRevision  string `yaml:"valuesRevision"`
	ExternalSecrets string `yaml:"externalSecrets"`
}

type Destination struct {
	Namespace string                     `yaml:"namespace"`
	Profile   string                     `yaml:"profile"`
	Clusters  map[string]ClusterOverride `yaml:"clusters"`
}

type ClusterOverride struct {
	Namespace       string `yaml:"namespace"`
	TargetRevision  string `yaml:"targetRevision"`
	ValuesRevision  string `yaml:"valuesRevision"`
	ExternalSecrets string `yaml:"externalSecrets"`
}

type ArgoCD struct {
	Namespace       string     `yaml:"namespace"`
	SyncWave        int        `yaml:"sync_wave"`
	CascadeDelete   bool       `yaml:"cascade_delete"`
	ServerSideApply bool       `yaml:"server_side_apply"`
	SkipDryRun      bool       `yaml:"skip_dry_run"`
	IgnoreDiff      IgnoreDiff `yaml:"ignore_diff"`
	Autosync        Autosync   `yaml:"autosync"`
}

type IgnoreDiff struct {
	Group string   `yaml:"group"`
	Kind  string   `yaml:"kind"`
	Paths []string `yaml:"paths"`
}

type Autosync struct {
	Enabled  bool `yaml:"enabled"`
	SelfHeal bool `yaml:"selfheal"`
	Prune    bool `yaml:"prune"`
}

type ConfigFile struct {
	Config  Config
	RelPath string
	Dir     string
}

func parseConfig(data []byte, relPath string) (Config, error) {
	var cfg Config
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)

	err := dec.Decode(&cfg)

	if err != nil {
		return Config{}, fmt.Errorf("%s: %w", relPath, err)
	}
	return cfg, nil
}

func validateConfig(cfg Config, relPath string) []error {
	var errs []error

	if (cfg.App.HelmChart.Name == "") == (cfg.App.HelmChart.Path == "") {
		errs = append(errs, fmt.Errorf("%s: exactly one of app.helmchart.name or app.helmchart.path must be set", relPath))
	}

	if cfg.App.Name == "" {
		errs = append(errs, fmt.Errorf("%s: app.name is required", relPath))
	} else if !checkRfc1123(cfg.App.Name) {
		errs = append(errs, fmt.Errorf("%s: app.name %q: not an RFC-1123 name", relPath, cfg.App.Name))
	}

	required := []struct{ val, key string }{
		{cfg.App.HelmChart.RepoURL, "app.helmchart.repoURL"},
		{cfg.App.HelmChart.Default.TargetRevision, "app.helmchart.default.targetRevision"},
		{cfg.App.HelmChart.Default.ValuesRevision, "app.helmchart.default.valuesRevision"},
		{cfg.App.Destination.Namespace, "app.destination.namespace"},
	}
	for _, r := range required {
		if r.val == "" {
			errs = append(errs, fmt.Errorf("%s: %s is required", relPath, r.key))
		}
	}
	if ns := cfg.ArgoCD.Namespace; ns != "" && (len(ns) > 63 || !checkRfc1123(ns)) {
		errs = append(errs, fmt.Errorf("%s: argocd.namespace %q: not a valid namespace name", relPath, ns))
	}

	return errs
}

func parseFlexBool(raw any, relPath string) (bool, string) {
	switch v := raw.(type) {
	case nil:
		return false, ""
	case bool:
		return v, ""
	case string:
		if strings.EqualFold(v, "true") {
			return true, ""
		}
		if strings.EqualFold(v, "false") {
			return false, ""
		}
	}
	return false, fmt.Sprintf("%s: externalSecretsEnabled %v: unrecognized value, treated as false", relPath, raw)
}

func (cf ConfigFile) SecretsEnabled() (bool, string) {
	return parseFlexBool(cf.Config.ExternalSecretsEnabled, cf.RelPath)
}

func argoNamespaceFor(cfg ConfigFile, s Settings) (string, error) {
	if ns := firstNonEmpty(cfg.Config.ArgoCD.Namespace, s.ArgoCD.Namespace); ns != "" {
		return ns, nil
	}
	return "", fmt.Errorf("%s: argocd.namespace is not set in the config or in hydrant.yaml", cfg.RelPath)
}
