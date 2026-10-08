package main

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"go.yaml.in/yaml/v3"
)

type clusterSecret struct {
	Kind     string `yaml:"kind"`
	Metadata struct {
		Labels      map[string]string `yaml:"labels"`
		Annotations map[string]string `yaml:"annotations"`
	} `yaml:"metadata"`
	StringData struct {
		Name string `yaml:"name"`
	} `yaml:"stringData"`
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

func parseClusterSecret(data []byte, relPath string) (Cluster, error) {
	var sec clusterSecret
	dec := yaml.NewDecoder(bytes.NewReader(data))
	err := dec.Decode(&sec)
	if err != nil {
		return Cluster{}, fmt.Errorf("%s: %w", relPath, err)
	}
	if sec.Kind != "Secret" {
		return Cluster{}, fmt.Errorf("%s: not a cluster secret: expected kind: Secret", relPath)
	}
	if sec.Metadata.Labels["argocd.argoproj.io/secret-type"] != "cluster" {
		return Cluster{}, fmt.Errorf("%s: not a cluster secret: expected label argocd.argoproj.io/secret-type=cluster", relPath)
	}
	if sec.StringData.Name == "" {
		return Cluster{}, fmt.Errorf("%s: stringData.name is empty or missing", relPath)
	} else if !checkRfc1123(sec.StringData.Name) {
		return Cluster{}, fmt.Errorf("%s: stringData.name must conform to RFC-1123", relPath)
	}

	c := Cluster{
		Name:        sec.StringData.Name,
		Labels:      sec.Metadata.Labels,
		Annotations: sec.Metadata.Annotations,
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
