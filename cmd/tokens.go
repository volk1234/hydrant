package main

import (
	"errors"
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strings"
)

var tokenRe = regexp.MustCompile(`\[[A-Z0-9_.-]+\]`)

func tokenName(key string, prefix string) string {
	t := strings.ToUpper(strings.TrimPrefix(key, prefix))
	t = "[" + t + "]"
	return t
}

func clusterTokens(c Cluster, prefix string) (map[string]string, error) {
	tokens := make(map[string]string)
	source := make(map[string]string)

	tokens["[CLUSTER_NAME]"] = c.Name
	source["[CLUSTER_NAME]"] = "cluster name"

	for _, k := range slices.Sorted(maps.Keys(c.Labels)) {
		if !strings.HasPrefix(k, prefix) {
			continue
		}
		token := tokenName(k, prefix)
		if _, exist := tokens[token]; exist {
			return nil, fmt.Errorf("cluster %q: token %q: defined by %s and label %q", c.Name, token, source[token], k)
		}
		tokens[token] = c.Labels[k]
		source[token] = fmt.Sprintf("label %q", k)
	}
	for _, k := range slices.Sorted(maps.Keys(c.Annotations)) {
		if !strings.HasPrefix(k, prefix) {
			continue
		}
		token := tokenName(k, prefix)
		if _, exist := tokens[token]; exist {
			return nil, fmt.Errorf("cluster %q: token %q: defined by %s and annotation %q", c.Name, token, source[token], k)
		}
		tokens[token] = c.Annotations[k]
		source[token] = fmt.Sprintf("annotation %q", k)
	}

	return tokens, nil
}

func substituteTokens(text string, c Cluster, prefix string) (string, error) {
	tokens, err := clusterTokens(c, prefix)
	if err != nil {
		return "", err
	}
	found := tokenRe.FindAllString(text, -1)
	slices.Sort(found)
	found = slices.Compact(found)

	var pairs []string
	var errs []error

	for _, t := range found {
		v, ok := tokens[t]
		if !ok || v == "" {
			errs = append(errs, fmt.Errorf("cluster %q: no value for token %q", c.Name, t))
			continue
		}
		pairs = append(pairs, t, v)
	}
	if len(errs) > 0 {
		return "", errors.Join(errs...)
	}

	return strings.NewReplacer(pairs...).Replace(text), nil
}
