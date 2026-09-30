package main

import (
	"fmt"
	"sort"
)

type Target struct {
	Cluster  Cluster
	Override ClusterOverride
}

type Intent struct {
	Config  ConfigFile
	Target  Target
	Secrets bool
}

func resolveTargets(cfg ConfigFile, clusters map[string]Cluster) ([]Target, []error, []string) {
	var errs []error
	var warns []string
	var names []string
	var result []Target
	targets := make(map[string]ClusterOverride)
	profile := cfg.Config.App.Destination.Profile
	overrides := cfg.Config.App.Destination.Clusters

	if profile != "" {
		label := profileLabel + profile
		for name, c := range clusters {
			if c.Labels[label] == "true" {
				targets[name] = ClusterOverride{}
			}
		}
		if len(targets) == 0 {
			warns = append(warns, fmt.Sprintf("%s: no target clusters: profile %q matched nothing", cfg.RelPath, profile))
		}
	}
	for name, ovr := range overrides {
		if _, exists := clusters[name]; !exists {
			errs = append(errs, fmt.Errorf("%s: app.destination.clusters: cluster %q not found in this environment", cfg.RelPath, name))
			continue
		}
		targets[name] = ovr
	}
	if len(targets) == 0 && len(errs) == 0 {
		warns = append(warns, fmt.Sprintf("%s: no target clusters: no explicit clusters are matched for the application", cfg.RelPath))
	}
	for name := range targets {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		result = append(result, Target{
			Cluster:  clusters[name],
			Override: targets[name],
		})
	}

	return result, errs, warns
}

func filterIntents(intents []Intent, render map[string]Cluster) []Intent {
	var result []Intent
	for _, it := range intents {
		if _, ok := render[it.Target.Cluster.Name]; ok {
			result = append(result, it)
		}
	}
	return result
}
