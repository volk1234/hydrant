package main

import (
	"flag"
	"fmt"
	"os"
	"regexp"
)

var rfc1123 = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`)
var version = "dev"

func checkRfc1123(s string) bool {
	return rfc1123.MatchString(s)
}

func main() {
	repo := flag.String("repo", "", "path to the checkout root")
	out := flag.String("out", "", "output path for rendered manifests")
	env := flag.String("env", "", "environment name(dev, live, ...)")
	project := flag.String("project", "default", "ArgoCD AppProject to target")
	partitionLabel := flag.String("partition-label", "", "cluster label to filter rendering by")
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Println("hydrant", version)
		os.Exit(0)
	}
	if *repo == "" || *out == "" || *project == "" {
		fmt.Fprintln(os.Stderr, "usage: hydrant -repo <path> -out <path> -env <name> ...")
		os.Exit(2)
	}
	if flag.NArg() > 0 {
		fmt.Fprintf(os.Stderr, "argument %q: hydrant takes no positional arguments\n", flag.Arg(0))
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
		fmt.Fprintf(os.Stderr, "-partition-label %q: no clusters match\n", *partitionLabel)
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
		fmt.Fprintln(os.Stderr, "warning:", w)
	}
	if len(cfgErrs) > 0 {
		failed = true
	}
	if len(configs) == 0 && len(cfgErrs) == 0 {
		fmt.Fprintf(os.Stderr, "%s: no %q files found\n", settings.Apps, "config-"+*env+".yaml")
		os.Exit(1)
	}
	var legacyCount int
	for _, c := range configs {
		if c.Config.LegacyEnabled == "true" {
			legacyCount++
		}
	}

	var intents []Intent
	brokenConfigs := make(map[string]bool)
	for _, cf := range configs {
		targets, errs, warns := resolveTargets(cf, clusters, settings.ProfileLabel())
		if len(errs) > 0 {
			failed = true
		}
		for _, e := range errs {
			fmt.Fprintln(os.Stderr, e)
		}
		for _, w := range warns {
			fmt.Fprintln(os.Stderr, "warning:", w)
		}
		secrets, warn := cf.SecretsEnabled()
		if warn != "" {
			fmt.Fprintln(os.Stderr, "warning:", warn)
		}

		for _, t := range targets {
			intents = append(intents, Intent{Config: cf, Target: t, Secrets: secrets})
		}
	}
	intents = filterIntents(intents, clustersToRender)
	intents = filterLegacy(intents)

	product := make(map[string][]byte)
	owners := make(map[string]string)

	for _, it := range intents {
		if brokenConfigs[it.Config.RelPath] {
			continue
		}
		ns, err := argoNamespaceFor(it.Config, settings)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			failed = true
			brokenConfigs[it.Config.RelPath] = true
			continue
		}
		values, err := buildValues(*repo, it.Config, it.Target, *env, settings.LabelPrefix)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			failed = true
			continue
		}

		app, err := buildApp(it.Config, it.Target, *env, *project, values, settings, ns)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			failed = true
			continue
		}
		if errs := checkLabelValues(app, it.Config.RelPath); len(errs) > 0 {
			for _, e := range errs {
				fmt.Fprintln(os.Stderr, e)
			}
			failed = true
			continue
		}
		appName := it.Config.Config.App.Name
		cluster := it.Target.Cluster.Name
		if err := addToProduct(product, owners, outputPath(cluster, appName), it.Config.RelPath, app); err != nil {
			fmt.Fprintln(os.Stderr, err)
			failed = true
			continue
		}

		if it.Secrets {
			sapp, err := buildSecretApp(it.Config, it.Target, *env, *project, settings, ns)
			if err != nil {
				fmt.Fprintln(os.Stderr, err)
				failed = true
				continue
			}
			if errs := checkLabelValues(sapp, it.Config.RelPath); len(errs) > 0 {
				for _, e := range errs {
					fmt.Fprintln(os.Stderr, e)
				}
				failed = true
				continue
			}
			if err := addToProduct(product, owners, outputPath(cluster, secretAppNamePrefix+appName), it.Config.RelPath, sapp); err != nil {
				fmt.Fprintln(os.Stderr, err)
				failed = true
				continue
			}
		}
	}

	fmt.Fprintf(os.Stderr, "%d configs, %d intents, %d legacy apps skipped, %d objects built\n", len(configs), len(intents), legacyCount, len(product))
	if failed {
		os.Exit(1)
	}

	if len(product) == 0 && legacyCount < len(configs) {
		fmt.Fprintln(os.Stderr, "product is empty: check -repo, -env and -partition-label")
		os.Exit(1)
	}
	if err = writeProduct(*out, *env, product); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Fprintf(os.Stderr, "rendered %d file(s)\n", len(product))

}
