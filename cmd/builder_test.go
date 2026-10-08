package main

import (
	"slices"
	"testing"
)

func TestBuildApp(t *testing.T) {
	cfg := ConfigFile{
		RelPath: "apps/my-app/config-dev.yaml",
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
	nsLabels := map[string]string{"readiness": "enabled"}
	argoNS := "argocd"

	app, err := buildApp(cfg, target, "dev", "default-proj", values, nsLabels, argoNS)
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

	// IgnoreDifferences
	if len(app.Spec.IgnoreDifferences) != 1 || app.Spec.IgnoreDifferences[0].Kind != "Deployment" {
		t.Errorf("IgnoreDifferences mismatch: %+v", app.Spec.IgnoreDifferences)
	}
}

func TestBuildSecretApp(t *testing.T) {
	cfg := ConfigFile{
		RelPath: "apps/karpenter/config-dev.yaml",
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

	secretsRepoURL := "git@github.com:org/secrets.git"
	appsDir := "cluster-init/apps"
	argoNS := "argocd"

	app, err := buildSecretApp(cfg, target, "dev", "default-proj", secretsRepoURL, appsDir, argoNS)
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
	if app.Spec.Source.RepoURL != secretsRepoURL {
		t.Errorf("RepoURL = %q, want %q", app.Spec.Source.RepoURL, secretsRepoURL)
	}
}
