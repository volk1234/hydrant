package main

import (
	"flag"
	"fmt"
	"os"
	"regexp"
	"strings"
)

const clustersDir = "generated/clusters"
const appsDir = "cluster-init/apps"
const profileLabel = "hydrant.net/profile-"

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
	strict := flag.Bool("strict", false, "treat warnings as errors")
	_ = strict
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

	clusters, err := loadClusters(*repo, *env)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	clustersToRender := filterByPartition(clusters, *partitionLabel)
	if len(clustersToRender) == 0 {
		fmt.Fprintf(os.Stderr, "no clusters match partition label %q\n", *partitionLabel)
		os.Exit(1)
	}
	fmt.Fprintf(os.Stderr, "%d clusters, %d to render\n", len(clusters), len(clustersToRender))

	configs, cfgErrs, err := loadConfigs(*repo, *env)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	for _, e := range cfgErrs {
		fmt.Fprintln(os.Stderr, e)
	}
	envs, err := listEnvs(*repo)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	stray, err := findStrayConfigs(*repo, envs)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	for _, w := range stray {
		fmt.Fprintln(os.Stderr, w)
	}
	if len(cfgErrs) > 0 {
		os.Exit(1)
	}
	if len(configs) == 0 {
		fmt.Fprintf(os.Stderr, "%s: no 'config-%s.yaml' files found\n", appsDir, *env)
		os.Exit(1)
	}

	failed := false
	var intents []Intent
	quarantined := make(map[string]bool)
	for _, cf := range configs {
		targets, errs, warns := resolveTargets(cf, clusters)
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
		// fmt.Fprintf(os.Stderr, "%s: %d targets\n", cf.Config.App.Name, len(targets))

		secrets, warn := cf.SecretsEnabled()
		if warn != "" {
			fmt.Fprintln(os.Stderr, warn)
		}

		for _, t := range targets {
			intents = append(intents, Intent{Config: cf, Target: t, Secrets: secrets})
		}
	}
	intents = filterIntents(intents, clustersToRender)

	var built []Application
	for _, it := range intents {

		values, err := buildValues(*repo, it.Config, it.Target, *env)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			failed = true
			quarantined[it.Config.RelPath] = true
			continue
		}

		_ = values

		app, err := buildApp(it.Config, it.Target, *env, *project)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			failed = true
			quarantined[it.Config.RelPath] = true
			continue
		}
		built = append(built, app)
		if it.Secrets {
			sapp, err := buildSecretApp(it.Config, it.Target, *env, *project)
			if err != nil {
				fmt.Fprintln(os.Stderr, err)
				failed = true
				quarantined[it.Config.RelPath] = true
				continue
			}
			built = append(built, sapp)
		}
	}

	for _, a := range built {
		if strings.HasPrefix(a.Metadata.Name, "test-local-both") {
			// doc, _ := marshalYAML(a)
			// os.Stdout.Write(doc)

		}
		// fmt.Printf("%s goes to %s\n", a.Metadata.Name, a.Spec.Destination.Name)
	}

	fmt.Fprintf(os.Stderr, "%d configs %d intents %d app quarantined %d apps build\n", len(configs), len(intents), len(quarantined), len(built))
	if failed {
		os.Exit(1)
	}

	fmt.Fprintln(os.Stderr, "rendered 0 file(s)")

}
