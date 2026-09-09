package main

import (
	"flag"
	"fmt"
	"os"
	"regexp"
)

const clustersDir = "generated/clusters"
const appsDir = "cluster-init/apps"

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
	check := flag.Bool("check", false, "check for drift, write nothing")

	_ = strict
	_ = check

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

	fmt.Fprintf(os.Stderr, "%d configs\n", len(configs))
	fmt.Fprintln(os.Stderr, "rendered 0 file(s)")
}
