package main

import (
	"bytes"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"

	"go.yaml.in/yaml/v3"
)

const settingsFileName = "hydrant.yaml"

var labelNameRe = regexp.MustCompile(`^[A-Za-z0-9]([-A-Za-z0-9_.]*[A-Za-z0-9])?$`)
var dnsSubdomainRe = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?(\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*$`)

type Settings struct {
	LabelPrefix    string         `yaml:"labelPrefix"`
	SecretsRepoURL string         `yaml:"secretsRepoURL"`
	Apps           string         `yaml:"apps"`
	Clusters       string         `yaml:"clusters"`
	ArgoCD         SettingsArgoCD `yaml:"argocd"`
}

type SettingsArgoCD struct {
	Namespace  string             `yaml:"namespace"`
	SyncPolicy SettingsSyncPolicy `yaml:"syncPolicy"`
}

type SettingsSyncPolicy struct {
	ManagedNamespaceMetadata struct {
		Labels map[string]string `yaml:"labels"`
	} `yaml:"managedNamespaceMetadata"`
}

func (s Settings) ProfileLabel() string {
	return s.LabelPrefix + "profile-"
}

func loadSettings(repo string) (Settings, []error) {
	var errs []error
	path := filepath.Join(repo, settingsFileName)

	data, err := os.ReadFile(path)
	if err != nil {
		return Settings{}, append(errs, fmt.Errorf("hydrant.yaml: cannot read config file(check -repo parameter): %w", err))
	}
	s, err := parseSettings(data)
	if err != nil {
		return Settings{}, append(errs, err)
	}
	if errs := validateSettings(s); len(errs) > 0 {
		return Settings{}, errs
	}
	return s, nil
}

func parseSettings(data []byte) (Settings, error) {

	var s Settings
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	err := dec.Decode(&s)
	if err != nil {
		return Settings{}, fmt.Errorf("'hydrant.yaml': %w", err)
	}
	return s, nil
}

func checkRepoPath(key, val string) []error {
	var errs []error
	if val == "" {
		return nil
	}
	if strings.HasPrefix(val, "/") {
		errs = append(errs, fmt.Errorf("hydrant.yaml: %s %q: must not start with /", key, val))
	}
	if strings.Contains(val, "\\") {
		errs = append(errs, fmt.Errorf("hydrant.yaml: %s %q: use / as separator, not \\", key, val))
	}
	if slices.Contains(strings.Split(val, "/"), "..") {
		errs = append(errs, fmt.Errorf("hydrant.yaml: %s %q: must not contain ..", key, val))
	}
	if clean := path.Clean(val); clean != val {
		errs = append(errs, fmt.Errorf("hydrant.yaml: %s %q: not in normal form, use %q", key, val, clean))
	}
	return errs
}

func validateSettings(s Settings) []error {
	var errs []error

	required := []struct{ val, key string }{
		{s.LabelPrefix, "labelPrefix"},
		{s.SecretsRepoURL, "secretsRepoURL"},
		{s.Apps, "apps"},
		{s.Clusters, "clusters"},
	}
	for _, r := range required {
		if r.val == "" {
			errs = append(errs, fmt.Errorf("hydrant.yaml: %s is required", r.key))
		}
	}

	if s.LabelPrefix != "" && !strings.HasSuffix(s.LabelPrefix, "/") {
		errs = append(errs, fmt.Errorf("hydrant.yaml: labelPrefix %q: must end with /", s.LabelPrefix))
	}
	appErrs := checkRepoPath("apps", s.Apps)
	clusterErrs := checkRepoPath("clusters", s.Clusters)
	errs = append(errs, appErrs...)
	errs = append(errs, clusterErrs...)
	if s.Apps != "" && s.Clusters != "" && len(appErrs) == 0 && len(clusterErrs) == 0 {
		if insideRepoPath(s.Apps, s.Clusters) || insideRepoPath(s.Clusters, s.Apps) {
			errs = append(errs, fmt.Errorf("hydrant.yaml: apps %q and clusters %q must not be the same directory or nested", s.Apps, s.Clusters))
		}
	}
	if ns := s.ArgoCD.Namespace; ns != "" && (len(ns) > 63 || !checkRfc1123(ns)) {
		errs = append(errs, fmt.Errorf("hydrant.yaml: argocd.namespace %q: not a valid namespace name", ns))
	}
	labels := s.ArgoCD.SyncPolicy.ManagedNamespaceMetadata.Labels
	keys := make([]string, 0, len(labels))
	for k := range labels {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if err := checkLabel(k, labels[k]); err != nil {
			errs = append(errs, fmt.Errorf("hydrant.yaml: argocd.syncPolicy.managedNamespaceMetadata.labels: %w", err))
		}
	}

	return errs
}

func checkPrefixInClusters(prefix string, clusters map[string]Cluster) error {
	for _, c := range clusters {
		for l := range c.Labels {
			if strings.HasPrefix(l, prefix) {
				return nil
			}
		}
		for a := range c.Annotations {
			if strings.HasPrefix(a, prefix) {
				return nil
			}
		}
	}
	return fmt.Errorf("hydrant.yaml: labelPrefix %q: no cluster in this environment has labels or annotations with this prefix", prefix)
}

func insideRepoPath(a, b string) bool {
	return a == b || strings.HasPrefix(a, b+"/")
}

func checkLabel(key, val string) error {

	prefix, name, found := strings.Cut(key, "/")
	if !found {
		name = prefix
		prefix = ""
	}
	if prefix != "" && (len(prefix) > 253 || !dnsSubdomainRe.MatchString(prefix)) {
		return fmt.Errorf("key %q: prefix %q is not a valid DNS subdomain", key, prefix)
	}
	if name == "" || len(name) > 63 || !labelNameRe.MatchString(name) {
		return fmt.Errorf("key %q: name %q is not a valid label name", key, name)
	}
	if val != "" && (len(val) > 63 || !labelNameRe.MatchString(val)) {
		return fmt.Errorf("key %q: value %q is not a valid label value", key, val)
	}
	return nil
}
