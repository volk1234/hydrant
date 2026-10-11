package main

import (
	"slices"
	"strings"
	"testing"
)

func TestBuildApp(t *testing.T) {
	cfg := ConfigFile{
		RelPath: "cluster-init/apps/my-app/config-dev.yaml",
		Config: Config{
			App: App{
				Name: "my-app",
				HelmChart: HelmChart{
					Name:        "my-chart",
					RepoURL:     "https://charts.example.com",
					ReleaseName: "custom-release",
					Default: ChartDefaults{
						TargetRevision: "1.0.0",
					},
				},
				Destination: Destination{
					Namespace: "default-ns",
				},
			},
			ArgoCD: ArgoCD{
				SyncWave:        5,
				CascadeDelete:   true,
				ServerSideApply: true,
				SkipDryRun:      true,
				Autosync: Autosync{
					Enabled:  true,
					SelfHeal: true,
					Prune:    true,
				},
				IgnoreDiff: IgnoreDiff{
					Group: "apps",
					Kind:  "Deployment",
					Paths: []string{"/spec/replicas"},
				},
			},
		},
	}

	target := Target{
		Cluster: Cluster{Name: "cluster-c-1"},
		Override: ClusterOverride{
			Namespace:      "override-ns",
			TargetRevision: "2.0.0",
		},
	}

	values := ValuesObject{}
	argoNS := "argocd"

	settings := Settings{
		LabelPrefix:    "hydrant.sh/",
		SecretsRepoURL: "git@github.com:org/secrets.git",
		Apps:           "cluster-init/apps",
		ArgoCD: SettingsArgoCD{
			SyncPolicy: SettingsSyncPolicy{
				ManagedNamespaceMetadata: struct {
					Labels map[string]string `yaml:"labels"`
				}{
					Labels: map[string]string{"readiness": "enabled"},
				},
			},
		},
	}

	app, err := buildApp(cfg, target, "dev", "default-proj", values, settings, argoNS)
	if err != nil {
		t.Fatalf("buildApp unexpected error: %v", err)
	}

	if app.Metadata.Name != "my-app-cluster-c-1" {
		t.Errorf("Metadata.Name = %q, want %q", app.Metadata.Name, "my-app-cluster-c-1")
	}
	if app.Metadata.Namespace != argoNS {
		t.Errorf("Metadata.Namespace = %q, want %q", app.Metadata.Namespace, argoNS)
	}
	if app.Metadata.Annotations["argocd.argoproj.io/sync-wave"] != "5" {
		t.Errorf("sync-wave annotation = %q, want 5", app.Metadata.Annotations["argocd.argoproj.io/sync-wave"])
	}
	if app.Metadata.Annotations["hydrant.sh/source-config"] != cfg.RelPath {
		t.Errorf("source-config annotation = %q, want %q", app.Metadata.Annotations["hydrant.sh/source-config"], cfg.RelPath)
	}
	if !slices.Contains(app.Metadata.Finalizers, resourcesFinalizer) {
		t.Errorf("Finalizers missing cascade delete finalizer: %v", app.Metadata.Finalizers)
	}

	// Target overrides
	if app.Spec.Source.TargetRevision != "2.0.0" {
		t.Errorf("TargetRevision = %q, want overridden 2.0.0", app.Spec.Source.TargetRevision)
	}
	if app.Spec.Destination.Namespace != "override-ns" {
		t.Errorf("Destination.Namespace = %q, want overridden override-ns", app.Spec.Destination.Namespace)
	}

	// Autosync
	if app.Spec.SyncPolicy.Automated == nil || !app.Spec.SyncPolicy.Automated.SelfHeal || !app.Spec.SyncPolicy.Automated.Prune {
		t.Errorf("Automated sync policy not properly configured: %+v", app.Spec.SyncPolicy.Automated)
	}

	// ManagedNamespaceMetadata
	if app.Spec.SyncPolicy.ManagedNamespaceMetadata == nil || app.Spec.SyncPolicy.ManagedNamespaceMetadata.Labels["readiness"] != "enabled" {
		t.Errorf("ManagedNamespaceMetadata mismatch: %+v", app.Spec.SyncPolicy.ManagedNamespaceMetadata)
	}

	// IgnoreDifferences
	if len(app.Spec.IgnoreDifferences) != 1 || app.Spec.IgnoreDifferences[0].Kind != "Deployment" {
		t.Errorf("IgnoreDifferences mismatch: %+v", app.Spec.IgnoreDifferences)
	}
}

func TestBuildSecretApp(t *testing.T) {
	cfg := ConfigFile{
		RelPath: "cluster-init/apps/karpenter/config-dev.yaml",
		Config: Config{
			App: App{
				Name: "karpenter",
				HelmChart: HelmChart{
					Default: ChartDefaults{
						ExternalSecrets: "main",
					},
				},
				Destination: Destination{
					Namespace: "karpenter-ns",
				},
			},
		},
	}

	target := Target{
		Cluster: Cluster{Name: "c1"},
	}

	settings := Settings{
		LabelPrefix:    "hydrant.sh/",
		SecretsRepoURL: "git@github.com:org/secrets.git",
		Apps:           "cluster-init/apps",
	}
	argoNS := "argocd"

	app, err := buildSecretApp(cfg, target, "dev", "default-proj", settings, argoNS)
	if err != nil {
		t.Fatalf("buildSecretApp unexpected error: %v", err)
	}

	expectedName := "external-secret-karpenter-c1"
	if app.Metadata.Name != expectedName {
		t.Errorf("Name = %q, want %q", app.Metadata.Name, expectedName)
	}
	expectedPath := "cluster-init/apps/karpenter/external_secrets/dev/"
	if app.Spec.Source.Path != expectedPath {
		t.Errorf("Source.Path = %q, want %q", app.Spec.Source.Path, expectedPath)
	}
	if app.Spec.Source.TargetRevision != "main" {
		t.Errorf("TargetRevision = %q, want %q", app.Spec.Source.TargetRevision, "main")
	}
	if app.Spec.Source.RepoURL != settings.SecretsRepoURL {
		t.Errorf("RepoURL = %q, want %q", app.Spec.Source.RepoURL, settings.SecretsRepoURL)
	}
	if app.Metadata.Annotations == nil {
		t.Fatal("expected non-nil Annotations")
	}
	if len(app.Metadata.Annotations) != 1 {
		t.Errorf("Annotations length = %d, want 1: %+v", len(app.Metadata.Annotations), app.Metadata.Annotations)
	}
	if app.Metadata.Annotations["hydrant.sh/source-config"] != cfg.RelPath {
		t.Errorf("source-config annotation = %q, want %q", app.Metadata.Annotations["hydrant.sh/source-config"], cfg.RelPath)
	}
}

func TestBuildAppCustomLabelPrefix(t *testing.T) {
	cfg := ConfigFile{
		RelPath: "cluster-init/apps/karpenter/config-dev.yaml",
		Config: Config{
			App: App{Name: "karpenter"},
		},
	}
	target := Target{Cluster: Cluster{Name: "c1"}}
	settings := Settings{
		LabelPrefix:    "example.org/",
		SecretsRepoURL: "git@github.com:org/secrets.git",
		Apps:           "cluster-init/apps",
	}

	app, err := buildApp(cfg, target, "dev", "default-proj", ValuesObject{}, settings, "argocd")
	if err != nil {
		t.Fatalf("buildApp unexpected error: %v", err)
	}
	if got := app.Metadata.Annotations["example.org/source-config"]; got != cfg.RelPath {
		t.Errorf("Annotations[example.org/source-config] = %q, want %q", got, cfg.RelPath)
	}
	if _, ok := app.Metadata.Annotations["hydrant.sh/source-config"]; ok {
		t.Errorf("Annotations unexpectedly contains hydrant.sh/source-config: %+v", app.Metadata.Annotations)
	}

	sapp, err := buildSecretApp(cfg, target, "dev", "default-proj", settings, "argocd")
	if err != nil {
		t.Fatalf("buildSecretApp unexpected error: %v", err)
	}
	if got := sapp.Metadata.Annotations["example.org/source-config"]; got != cfg.RelPath {
		t.Errorf("Annotations[example.org/source-config] = %q, want %q", got, cfg.RelPath)
	}
	if _, ok := sapp.Metadata.Annotations["hydrant.sh/source-config"]; ok {
		t.Errorf("Annotations unexpectedly contains hydrant.sh/source-config: %+v", sapp.Metadata.Annotations)
	}
}

func TestMarshalYAML(t *testing.T) {
	app := Application{
		APIVersion: appAPIVersion,
		Kind:       appKind,
		Metadata:   Metadata{Name: "my-app"},
	}
	bytes, err := marshalYAML(app)
	if err != nil {
		t.Fatalf("marshalYAML failed: %v", err)
	}
	if len(bytes) == 0 {
		t.Fatal("expected non-empty YAML bytes")
	}
}

func TestNamespaceMetadataFromEmpty(t *testing.T) {
	res := namespaceMetadataFrom(nil)
	if res != nil {
		t.Errorf("namespaceMetadataFrom(nil) = %v, want nil", res)
	}
}

func TestCheckLabelValues(t *testing.T) {
	relPath := "cluster-init/apps/karpenter/config-dev.yaml"
	objName := "test-object"

	t.Run("boundary_63_chars", func(t *testing.T) {
		app := Application{
			Metadata: Metadata{
				Name: objName,
				Labels: map[string]string{
					"my-label": strings.Repeat("a", 63),
				},
			},
		}
		errs := checkLabelValues(app, relPath)
		if len(errs) != 0 {
			t.Errorf("expected 0 errors, got %d: %v", len(errs), errs)
		}
	})

	t.Run("over_boundary_64_chars", func(t *testing.T) {
		labelKey := "long-label"
		app := Application{
			Metadata: Metadata{
				Name: objName,
				Labels: map[string]string{
					labelKey: strings.Repeat("a", 64),
				},
			},
		}
		errs := checkLabelValues(app, relPath)
		if len(errs) != 1 {
			t.Fatalf("expected 1 error, got %d: %v", len(errs), errs)
		}
		msg := errs[0].Error()
		if !strings.Contains(msg, objName) {
			t.Errorf("error %q should contain object name %q", msg, objName)
		}
		if !strings.Contains(msg, labelKey) {
			t.Errorf("error %q should contain label key %q", msg, labelKey)
		}
		if !strings.Contains(msg, "64") {
			t.Errorf("error %q should contain '64'", msg)
		}
	})

	t.Run("two_long_labels_sorted", func(t *testing.T) {
		app := Application{
			Metadata: Metadata{
				Name: objName,
				Labels: map[string]string{
					"b": strings.Repeat("b", 64),
					"a": strings.Repeat("a", 64),
				},
			},
		}
		errs := checkLabelValues(app, relPath)
		if len(errs) != 2 {
			t.Fatalf("expected 2 errors, got %d: %v", len(errs), errs)
		}
		if !strings.Contains(errs[0].Error(), `label "a":`) {
			t.Errorf("first error should be about label a, got: %q", errs[0].Error())
		}
		if !strings.Contains(errs[1].Error(), `label "b":`) {
			t.Errorf("second error should be about label b, got: %q", errs[1].Error())
		}
	})
}
