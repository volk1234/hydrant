package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"

	"go.yaml.in/yaml/v3"
)

const settingsFileName = "hydrant.yaml"

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

func loadSettings(repo string) (Settings, error) {
	path := filepath.Join(repo, settingsFileName)

	data, err := os.ReadFile(path)
	if err != nil {
		return Settings{}, fmt.Errorf("%s: cannot read config file(check -repo parameter): %w", path, err)
	}
	s, err := parseSettings(data)
	if err != nil {
		return Settings{}, err
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
