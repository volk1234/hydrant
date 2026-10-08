package main

import (
	"flag"
	"fmt"
	"os"
	"regexp"
)

var rfc1123 = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`)

func checkRfc1123(s string) bool {
	return rfc1123.MatchString(s)
}

func main() {
	repo := flag.String("repo", "", "path to the checkout root")
	out := flag.String("out", "", "output path for rendered manifests")
	env := flag.String("env", "", "environment name(dev, live, ...)")
	project := flag.String("project", "default", "ArgoCD AppProject to target")
	partitionLabel := flag.String("partition-label", "", "cluster label to filter rendering by")
	flag.Parse()

	if *repo == "" || *out == "" || *project == "" {
		fmt.Fprintln(os.Stderr, "usage: hydrant -repo <path> -out <path> -env <name> ...")
		os.Exit(2)
	}
	if flag.NArg() > 0 {
		fmt.Fprintf(os.Stderr, "unexpected argument %q\n", flag.Arg(0))
		os.Exit(2)
	}
	if !checkRfc1123(*env) {
		fmt.Fprintf(os.Stderr, "-env %q: must be a non-empty RFC-1123 name (lowercase letters, digits, hyphens)\n", *env)
		os.Exit(2)
	}

	settings, errs := loadSettings(*repo)
	if len(errs) > 0 {
		for _, e := range errs {
			fmt.Fprintln(os.Stderr, e)
		}
		os.Exit(1)
	}

	if err := checkOutDir(*repo, *out, settings.Apps, settings.Clusters); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}

	clusters, err := loadClusters(*repo, *env, settings.Clusters)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := checkPrefixInClusters(settings.LabelPrefix, clusters); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	clustersToRender := filterByPartition(clusters, *partitionLabel)
	if len(clustersToRender) == 0 {
		fmt.Fprintf(os.Stderr, "no clusters match partition label %q\n", *partitionLabel)
		os.Exit(1)
	}
	fmt.Fprintf(os.Stderr, "%d clusters, %d to render\n", len(clusters), len(clustersToRender))

	failed := false
	configs, cfgErrs, err := loadConfigs(*repo, *env, settings.Apps)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	for _, e := range cfgErrs {
		fmt.Fprintln(os.Stderr, e)
	}
	envs, err := listEnvs(*repo, settings.Clusters)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	stray, err := findStrayConfigs(*repo, envs, settings.Apps)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	for _, w := range stray {
		fmt.Fprintln(os.Stderr, w)
	}
	if len(cfgErrs) > 0 {
		failed = true
	}
	if len(configs) == 0 && len(cfgErrs) == 0 {
		fmt.Fprintf(os.Stderr, "%s: no 'config-%s.yaml' files found\n", settings.Apps, *env)
		os.Exit(1)
	}

	var intents []Intent
	quarantined := make(map[string]bool)
	for _, cf := range configs {
		targets, errs, warns := resolveTargets(cf, clusters, settings.ProfileLabel())
		if len(errs) > 0 {
			failed = true
			quarantined[cf.RelPath] = true
		}
		for _, e := range errs {
			fmt.Fprintln(os.Stderr, e)
		}
		for _, w := range warns {
			fmt.Fprintln(os.Stderr, w)
		}
		secrets, warn := cf.SecretsEnabled()
		if warn != "" {
			fmt.Fprintln(os.Stderr, warn)
		}

		for _, t := range targets {
			intents = append(intents, Intent{Config: cf, Target: t, Secrets: secrets})
		}
	}
	intents = filterIntents(intents, clustersToRender)
	intents, legacy := filterLegacy(intents)
	for _, e := range checkCollisions(intents) {
		fmt.Fprintln(os.Stderr, e)
		failed = true
	}

	product := make(map[string][]byte)

	for _, it := range intents {
		if quarantined[it.Config.RelPath] {
			continue
		}
		ns, err := argoNamespaceFor(it.Config, settings)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			failed = true
			quarantined[it.Config.RelPath] = true
			continue
		}
		values, err := buildValues(*repo, it.Config, it.Target, *env, settings.LabelPrefix)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			failed = true
			quarantined[it.Config.RelPath] = true
			continue
		}

		app, err := buildApp(it.Config, it.Target, *env, *project, values, settings.ArgoCD.SyncPolicy.ManagedNamespaceMetadata.Labels, ns)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			failed = true
			quarantined[it.Config.RelPath] = true
			continue
		}
		appName := it.Config.Config.App.Name
		cluster := it.Target.Cluster.Name
		if err := addToProduct(product, outputPath(cluster, appName), app); err != nil {
			fmt.Fprintln(os.Stderr, err)
			failed = true
			continue
		}

		if it.Secrets {
			sapp, err := buildSecretApp(it.Config, it.Target, *env, *project, settings.SecretsRepoURL, settings.Apps, ns)
			if err != nil {
				fmt.Fprintln(os.Stderr, err)
				failed = true
				quarantined[it.Config.RelPath] = true
				continue
			}
			if err := addToProduct(product, outputPath(cluster, secretAppNamePrefix+appName), sapp); err != nil {
				fmt.Fprintln(os.Stderr, err)
				failed = true
				continue
			}
		}
	}

	fmt.Fprintf(os.Stderr, "%d configs %d intents %d app quarantined  %d legacy_skipped %d apps build\n", len(configs), len(intents), len(quarantined), legacy, len(product))
	if failed {
		os.Exit(1)
	}

	if len(product) == 0 && legacy < len(configs) {
		fmt.Fprintln(os.Stderr, "product is empty: check -repo, -env and -partition-label")
		os.Exit(1)
	}
	if err = writeProduct(*out, *env, product); err != nil {
		fmt.Fprintln(os.Stderr, "write files on disk: ", err)
		os.Exit(1)
	}
	fmt.Fprintf(os.Stderr, "rendered %d file(s)\n", len(product))

}
