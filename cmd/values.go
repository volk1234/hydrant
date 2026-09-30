package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"go.yaml.in/yaml/v3"
)

type valuesLayer struct {
	RelPath string
	Data    []byte
}

type pair struct {
	key, val *yaml.Node
}

type ValuesObject struct {
	node *yaml.Node
}

func (v ValuesObject) IsZero() bool {
	return v.node == nil || len(v.node.Content) == 0
}

func (v ValuesObject) MarshalYAML() (any, error) {
	return v.node, nil
}

func readValuesLayers(repo, dir, env, cluster string) ([]valuesLayer, error) {
	names := []string{
		"values.yaml",
		"values." + env + ".yaml",
		"values." + cluster + ".yaml",
	}
	var layers []valuesLayer
	for _, name := range names {
		relPath := dir + "/" + name
		path := filepath.Join(repo, filepath.FromSlash(relPath))
		data, err := os.ReadFile(path)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("%s: %w", relPath, err)
		}

		layers = append(layers, valuesLayer{RelPath: relPath, Data: data})

	}
	return layers, nil
}

func mergeValues(lower, upper *yaml.Node) *yaml.Node {
	if lower == nil {
		return upper
	}

	result := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	result.Content = append(result.Content, lower.Content...)

	for i := 0; i < len(upper.Content); i += 2 {
		key := upper.Content[i]
		uv := upper.Content[i+1]
		j := findKey(result, key.Value)
		if j < 0 {
			result.Content = append(result.Content, key, uv)
			continue
		}
		lv := result.Content[j+1]
		if lv.Kind == yaml.MappingNode && uv.Kind == yaml.MappingNode {
			result.Content[j+1] = mergeValues(lv, uv)
		} else {
			result.Content[j+1] = uv
		}
	}
	return result
}

func parseValues(data []byte, relPath string) (*yaml.Node, error) {
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("%s: %w", relPath, err)
	}
	if len(doc.Content) == 0 {
		return nil, nil
	}
	root := doc.Content[0]

	if root.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("%s: root of a values file must be a mapping", relPath)
	}

	return root, nil
}

func findKey(m *yaml.Node, key string) int {
	for i := 0; i < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			return i
		}
	}
	return -1
}

func normalizeValues(n *yaml.Node) {
	if n == nil {
		return
	}
	switch n.Kind {
	case yaml.MappingNode:
		var pairs []pair
		for i := 0; i < len(n.Content); i += 2 {
			pairs = append(pairs, pair{n.Content[i], n.Content[i+1]})
		}
		sort.Slice(pairs, func(a, b int) bool {
			return pairs[a].key.Value < pairs[b].key.Value
		})
		n.Content = n.Content[:0]
		for _, p := range pairs {
			normalizeValues(p.val)
			n.Content = append(n.Content, p.key, p.val)
		}
	case yaml.SequenceNode:
		for _, item := range n.Content {
			normalizeValues(item)
		}
	}
}

func buildValues(repo string, cfg ConfigFile, t Target, env string) (ValuesObject, error) {

	// read 3 values files
	valuesFiles, err := readValuesLayers(repo, cfg.Dir, env, t.Cluster.Name)
	if err != nil {
		return ValuesObject{}, err
	}

	// append inline tokens
	if cfg.Config.InlineValues != "" {
		text, err := substituteTokens(cfg.Config.InlineValues, t.Cluster)
		if err != nil {
			return ValuesObject{}, fmt.Errorf("%s: %w", cfg.RelPath, err)
		}
		valuesFiles = append(valuesFiles, valuesLayer{RelPath: cfg.RelPath + ": inline_values", Data: []byte(text)})
	}

	var result *yaml.Node
	for _, l := range valuesFiles {
		// convert data to node tree
		n, err := parseValues(l.Data, l.RelPath)
		if err != nil {
			return ValuesObject{}, err
		}
		if n == nil {
			continue
		}
		// merge values
		result = mergeValues(result, n)
		//merge rules:
		// no key - append to the end
		// is string/is number - replace
		//is mapingnode? mergeValues recusively
	}
	normalizeValues(result)

	return ValuesObject{node: result}, nil
}
