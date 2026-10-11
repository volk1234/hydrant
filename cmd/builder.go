package main

import (
	"bytes"
	"fmt"
	"maps"
	"slices"
	"strconv"

	"go.yaml.in/yaml/v3"
)

const resourcesFinalizer = "resources-finalizer.argocd.argoproj.io"
const appAPIVersion = "argoproj.io/v1alpha1"
const appKind = "Application"
const skipSchemaValidation = true
const createNamespace = true
const kubeAppLabel = "app.kubernetes.io/name"
const chartParentPrefix = "cluster-init-"
const secretAppPathSuffix = "external_secrets"
const secretAppNamePrefix = "external-secret-"
const secretAppNamePluralPrefix = "external-secrets-"
const maxLabelValueLen = 63

type Application struct {
	APIVersion string   `yaml:"apiVersion"`
	Kind       string   `yaml:"kind"`
	Metadata   Metadata `yaml:"metadata"`
	Spec       Spec     `yaml:"spec"`
}

type Metadata struct {
	Name        string            `yaml:"name"`
	Namespace   string            `yaml:"namespace"`
	Annotations map[string]string `yaml:"annotations,omitempty"`
	Labels      map[string]string `yaml:"labels"`
	Finalizers  []string          `yaml:"finalizers,omitempty"`
}

type Spec struct {
	Project           string              `yaml:"project"`
	Source            *Source             `yaml:"source,omitempty"`
	Destination       AppDestination      `yaml:"destination"`
	IgnoreDifferences []IgnoreDifferences `yaml:"ignoreDifferences,omitempty"`
	SyncPolicy        SyncPolicy          `yaml:"syncPolicy"`
}

type Source struct {
	RepoURL        string `yaml:"repoURL"`
	Chart          string `yaml:"chart,omitempty"`
	Path           string `yaml:"path,omitempty"`
	TargetRevision string `yaml:"targetRevision"`
	Helm           *Helm  `yaml:"helm,omitempty"`
}

type Helm struct {
	ReleaseName          string       `yaml:"releaseName"`
	SkipSchemaValidation bool         `yaml:"skipSchemaValidation"`
	ValuesObject         ValuesObject `yaml:"valuesObject,omitempty"`
}

type AppDestination struct {
	Name      string `yaml:"name"`
	Namespace string `yaml:"namespace"`
}

type IgnoreDifferences struct {
	Group             string   `yaml:"group"`
	Kind              string   `yaml:"kind"`
	JQPathExpressions []string `yaml:"jqPathExpressions"`
}

type SyncPolicy struct {
	ManagedNamespaceMetadata *ManagedNamespaceMetadata `yaml:"managedNamespaceMetadata,omitempty"`
	SyncOptions              []string                  `yaml:"syncOptions"`
	Automated                *Automated                `yaml:"automated,omitempty"`
}

type ManagedNamespaceMetadata struct {
	Labels map[string]string `yaml:"labels"`
}

type Automated struct {
	Enabled  bool `yaml:"enabled"`
	SelfHeal bool `yaml:"selfHeal"`
	Prune    bool `yaml:"prune"`
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

func marshalYAML(app Application) ([]byte, error) {
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(app); err != nil {
		return nil, err
	}
	if err := enc.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func autosyncFrom(a Autosync) *Automated {
	if !a.Enabled {
		return nil
	}
	return &Automated{Enabled: a.Enabled, SelfHeal: a.SelfHeal, Prune: a.Prune}
}

func namespaceMetadataFrom(labels map[string]string) *ManagedNamespaceMetadata {
	if len(labels) == 0 {
		return nil
	}
	return &ManagedNamespaceMetadata{Labels: labels}
}

func checkLabelValues(app Application, relPath string) []error {
	var errs []error
	keys := slices.Sorted(maps.Keys(app.Metadata.Labels))
	for _, k := range keys {
		v := app.Metadata.Labels[k]
		if len(v) > maxLabelValueLen {
			errs = append(errs, fmt.Errorf("%s: object %q: label %q: value is %d characters, limit is %d", relPath, app.Metadata.Name, k, len(v), maxLabelValueLen))
		}
	}
	return errs
}

func buildApp(cfg ConfigFile, t Target, env, project string, values ValuesObject, s Settings, ns string) (Application, error) {
	name := cfg.Config.App.Name + "-" + t.Cluster.Name

	app := Application{
		APIVersion: appAPIVersion,
		Kind:       appKind,
		Metadata: Metadata{
			Name:      name,
			Namespace: ns,
			Annotations: map[string]string{
				"argocd.argoproj.io/sync-wave": strconv.Itoa(cfg.Config.ArgoCD.SyncWave),
				s.SourceConfigAnnotation():     cfg.RelPath,
			},
			Labels: map[string]string{
				kubeAppLabel: name,
				"env":        env,
				"parent-app": chartParentPrefix + env,
			},
		},
		Spec: Spec{
			Project: project,
			Source: &Source{

				RepoURL:        cfg.Config.App.HelmChart.RepoURL,
				Chart:          cfg.Config.App.HelmChart.Name,
				Path:           cfg.Config.App.HelmChart.Path,
				TargetRevision: firstNonEmpty(t.Override.TargetRevision, cfg.Config.App.HelmChart.Default.TargetRevision),
				Helm: &Helm{
					ReleaseName:          firstNonEmpty(cfg.Config.App.HelmChart.ReleaseName, name),
					SkipSchemaValidation: skipSchemaValidation,
					ValuesObject:         values,
				},
			},
			Destination: AppDestination{
				Name:      t.Cluster.Name,
				Namespace: firstNonEmpty(t.Override.Namespace, cfg.Config.App.Destination.Namespace),
			},
			SyncPolicy: SyncPolicy{
				ManagedNamespaceMetadata: namespaceMetadataFrom(s.ArgoCD.SyncPolicy.ManagedNamespaceMetadata.Labels),
				SyncOptions: []string{
					"ServerSideApply=" + strconv.FormatBool(cfg.Config.ArgoCD.ServerSideApply),
					"CreateNamespace=" + strconv.FormatBool(createNamespace),
					"SkipDryRunOnMissingResource=" + strconv.FormatBool(cfg.Config.ArgoCD.SkipDryRun),
				},
				Automated: autosyncFrom(cfg.Config.ArgoCD.Autosync),
			},
		},
	}
	if cfg.Config.ArgoCD.CascadeDelete {
		app.Metadata.Finalizers = []string{resourcesFinalizer}
	}
	if cfg.Config.ArgoCD.IgnoreDiff.Group != "" || cfg.Config.ArgoCD.IgnoreDiff.Kind != "" {
		app.Spec.IgnoreDifferences = []IgnoreDifferences{{
			Group:             cfg.Config.ArgoCD.IgnoreDiff.Group,
			Kind:              cfg.Config.ArgoCD.IgnoreDiff.Kind,
			JQPathExpressions: cfg.Config.ArgoCD.IgnoreDiff.Paths,
		}}
	}

	return app, nil
}

func buildSecretApp(cfg ConfigFile, t Target, env, project string, s Settings, ns string) (Application, error) {
	name := secretAppNamePrefix + cfg.Config.App.Name + "-" + t.Cluster.Name
	secretsPath := s.Apps + "/" + cfg.Config.App.Name + "/" + secretAppPathSuffix + "/" + env + "/"

	app := Application{
		APIVersion: appAPIVersion,
		Kind:       appKind,
		Metadata: Metadata{
			Name:      name,
			Namespace: ns,
			Annotations: map[string]string{
				s.SourceConfigAnnotation(): cfg.RelPath,
			},
			Labels: map[string]string{
				kubeAppLabel: name,
				"env":        env,
				"parent-app": secretAppNamePluralPrefix + env,
			},
		},
		Spec: Spec{
			Project: project,
			Source: &Source{
				RepoURL:        s.SecretsRepoURL,
				Path:           secretsPath,
				TargetRevision: firstNonEmpty(t.Override.ExternalSecrets, cfg.Config.App.HelmChart.Default.ExternalSecrets),
			},
			Destination: AppDestination{
				Name:      t.Cluster.Name,
				Namespace: firstNonEmpty(t.Override.Namespace, cfg.Config.App.Destination.Namespace),
			},
			SyncPolicy: SyncPolicy{
				SyncOptions: []string{
					"CreateNamespace=" + strconv.FormatBool(createNamespace),
				},
				Automated: autosyncFrom(cfg.Config.ArgoCD.Autosync),
			},
		},
	}
	return app, nil
}
