package main

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
)

const tokenPrefix = "hydrant.sh/"

var tokenRe = regexp.MustCompile(`\[[A-Z0-9_]+\]`)

func tokenName(key string) string {
	t := strings.ToUpper(strings.TrimPrefix(key, tokenPrefix))
	t = "[" + t + "]"
	return t
}

func clusterTokens(c Cluster) (map[string]string, error) {
	tokens := make(map[string]string)

	tokens["[CLUSTER_NAME]"] = c.Name
	for k, v := range c.Labels {
		if !strings.HasPrefix(k, tokenPrefix) {
			continue
		}
		token := tokenName(k)
		if _, exist := tokens[token]; exist {
			return nil, fmt.Errorf(" cluster %q: label %s: token %s is already defined", c.Name, k, token)
		}
		tokens[token] = v
	}
	for k, v := range c.Annotations {
		if !strings.HasPrefix(k, tokenPrefix) {
			continue
		}
		token := tokenName(k)
		if _, exist := tokens[token]; exist {
			return nil, fmt.Errorf(" cluster %q: annotation %s: token %s is already defined", c.Name, k, token)
		}
		tokens[token] = v
	}

	return tokens, nil
}

func substituteTokens(text string, c Cluster) (string, error) {
	tokens, err := clusterTokens(c)
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
			errs = append(errs, fmt.Errorf("cluster %q has no value for token %s", c.Name, t))
			continue
		}
		pairs = append(pairs, t, v)
	}
	if len(errs) > 0 {
		return "", errors.Join(errs...)
	}

	return strings.NewReplacer(pairs...).Replace(text), nil
}
