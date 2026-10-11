package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"go.yaml.in/yaml/v3"
)

type objectMeta struct {
	Labels      map[string]string `yaml:"labels"`
	Annotations map[string]string `yaml:"annotations"`
}
type secretFile struct {
	Metadata   objectMeta `yaml:"metadata"`
	StringData struct {
		Name string `yaml:"name"`
	} `yaml:"stringData"`
}

type esoSecretFile struct {
	Metadata objectMeta `yaml:"metadata"`
	Spec     struct {
		Target struct {
			Template struct {
				Metadata     objectMeta `yaml:"metadata"`
				TemplateFrom []any      `yaml:"templateFrom"`
				Data         struct {
					Name string `yaml:"name"`
				} `yaml:"data"`
			} `yaml:"template"`
		} `yaml:"target"`
	} `yaml:"spec"`
}

type Cluster struct {
	Name        string
	Labels      map[string]string
	Annotations map[string]string
}

func listEnvs(repo, clustersDir string) ([]string, error) {
	entries, err := os.ReadDir(filepath.Join(repo, clustersDir))
	if err != nil {
		return nil, fmt.Errorf("%s: %w", clustersDir, err)
	}
	var envs []string
	for _, entry := range entries {
		if entry.IsDir() {
			envs = append(envs, entry.Name())
		}
	}
	return envs, nil
}

func loadClusters(repo, env, clustersDir string) (map[string]Cluster, error) {
	dir := filepath.Join(repo, clustersDir, env)
	if _, err := os.Stat(dir); errors.Is(err, os.ErrNotExist) {

		envs, lerr := listEnvs(repo, clustersDir)
		if lerr != nil {
			return nil, lerr
		}
		available := "no environments found"
		if len(envs) > 0 {
			available = strings.Join(envs, ", ")
		}
		return nil, fmt.Errorf("%s/%s: environment directory not found; available environments: %s", clustersDir, env, available)
	} else if err != nil {
		return nil, fmt.Errorf("%s/%s: %w", clustersDir, env, err)
	}
	files, err := filepath.Glob(filepath.Join(dir, "*.yaml"))
	if err != nil {
		return nil, fmt.Errorf("%s/%s: %w", clustersDir, env, err)
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("%s/%s: no cluster secrets found", clustersDir, env)
	}
	clusters := make(map[string]Cluster)
	for _, f := range files {
		relPath, err := filepath.Rel(repo, f)
		if err != nil {
			return nil, fmt.Errorf("%s: cannot compute path relative to repository root: %w", f, err)
		}
		relPath = filepath.ToSlash(relPath)
		data, err := os.ReadFile(f)
		if err != nil {
			return nil, fmt.Errorf("%s: cannot read file: %w", relPath, err)
		}

		c, err := parseClusterSecret(data, relPath)
		if err != nil {
			return nil, err
		}
		if _, exists := clusters[c.Name]; exists {
			return nil, fmt.Errorf("%s: duplicate cluster name %q", relPath, c.Name)
		}
		clusters[c.Name] = c

	}
	return clusters, nil
}

func hasTemplate(m map[string]string) bool {
	for k, v := range m {
		if strings.Contains(k, "{{") || strings.Contains(v, "{{") {
			return true
		}
	}
	return false
}

func parseClusterSecret(data []byte, relPath string) (Cluster, error) {
	var name string
	var meta objectMeta
	var head struct {
		Kind string `yaml:"kind"`
	}
	nameField := "stringData.name"
	if err := yaml.Unmarshal(data, &head); err != nil {
		return Cluster{}, fmt.Errorf("%s: %w", relPath, err)
	}

	switch head.Kind {
	case "Secret":
		var s secretFile
		if err := yaml.Unmarshal(data, &s); err != nil {
			return Cluster{}, fmt.Errorf("%s: %w", relPath, err)
		}
		name, meta = s.StringData.Name, s.Metadata
	case "ExternalSecret":
		var es esoSecretFile
		if err := yaml.Unmarshal(data, &es); err != nil {
			return Cluster{}, fmt.Errorf("%s: %w", relPath, err)
		}
		t := es.Spec.Target.Template
		if len(t.TemplateFrom) > 0 {
			return Cluster{}, fmt.Errorf("%s: ExternalSecret: spec.target.template.templateFrom is not supported", relPath)
		}
		if es.Metadata.Labels["argocd.argoproj.io/secret-type"] == "cluster" && t.Metadata.Labels["argocd.argoproj.io/secret-type"] != "cluster" {
			return Cluster{}, fmt.Errorf("%s: ExternalSecret: label argocd.argoproj.io/secret-type is in metadata.labels; move it to spec.target.template.metadata.labels", relPath)
		}

		if strings.Contains(t.Data.Name, "{{") || hasTemplate(t.Metadata.Labels) || hasTemplate(t.Metadata.Annotations) {
			return Cluster{}, fmt.Errorf("%s: ExternalSecret: name, labels and annotations must be plain values: hydrant cannot read values the operator fills in ({{ }})", relPath)
		}

		name, meta = t.Data.Name, t.Metadata
		nameField = "spec.target.template.data.name"

	default:
		return Cluster{}, fmt.Errorf("%s: not a cluster secret: expected kinds: Secret or ExternalSecret but got: %q", relPath, head.Kind)
	}

	if meta.Labels["argocd.argoproj.io/secret-type"] != "cluster" {
		return Cluster{}, fmt.Errorf("%s: not a cluster secret: expected label argocd.argoproj.io/secret-type=cluster", relPath)
	}
	if name == "" {
		return Cluster{}, fmt.Errorf("%s: %s is empty or missing", relPath, nameField)
	} else if !checkRfc1123(name) {
		return Cluster{}, fmt.Errorf("%s: %s must conform to RFC-1123", relPath, nameField)
	}

	c := Cluster{
		Name:        name,
		Labels:      meta.Labels,
		Annotations: meta.Annotations,
	}
	return c, nil
}

func filterByPartition(clusters map[string]Cluster, label string) map[string]Cluster {
	if label == "" {
		return clusters
	}
	selected := make(map[string]Cluster)
	for name, c := range clusters {
		if c.Labels[label] == "true" {
			selected[name] = c
		}
	}
	return selected
}
