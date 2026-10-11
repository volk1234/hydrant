package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func checkCollisions(intents []Intent) []error {
	owner := make(map[string]string)
	var errs []error
	for _, it := range intents {
		key := it.Target.Cluster.Name + "/" + it.Config.Config.App.Name
		if prev, ok := owner[key]; ok {
			errs = append(errs, fmt.Errorf("%s/%s.yaml: claimed by %q and %q", it.Target.Cluster.Name, it.Config.Config.App.Name, prev, it.Config.RelPath))
			continue
		}
		owner[key] = it.Config.RelPath

	}
	return errs
}

func outputPath(cluster, file string) string {
	return cluster + "/" + file + ".yaml"
}

func addToProduct(product map[string][]byte, path string, app Application) error {
	doc, err := marshalYAML(app)
	if err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	product[path] = doc
	return nil
}

func canonical(p string) (string, error) {
	abs, err := filepath.Abs(p)
	if err != nil {
		return "", fmt.Errorf("%s: %w", p, err)
	}
	existing := abs
	tail := ""
	for {
		if _, err := os.Stat(existing); err == nil {
			break
		}
		tail = filepath.Join(filepath.Base(existing), tail)
		existing = filepath.Dir(existing)
	}
	real, err := filepath.EvalSymlinks(existing)
	if err != nil {
		return "", fmt.Errorf("%s: %w", existing, err)
	}

	return filepath.Join(real, tail), nil
}

func within(a, b string) bool {
	return a == b || strings.HasPrefix(a, b+string(filepath.Separator))
}

func checkOutDir(repo, out, appsDir, clustersDir string) error {
	repo, err := canonical(repo)
	if err != nil {
		return fmt.Errorf("-repo: %w", err)
	}
	out, err = canonical(out)
	if err != nil {
		return fmt.Errorf("-out: %w", err)
	}
	if within(repo, out) {
		return fmt.Errorf("-out %q: is the checkout itself or above it", out)
	}
	for _, dir := range []string{appsDir, clustersDir} {
		d := filepath.Join(repo, dir)
		if within(out, d) || within(d, out) {
			return fmt.Errorf("-out %q: overlaps input directory %q", out, d)
		}
	}

	return nil
}

func writeProduct(out, env string, product map[string][]byte) error {
	if err := os.MkdirAll(out, 0o755); err != nil {
		return fmt.Errorf("create output directory: %w", err)
	}
	dir, err := os.MkdirTemp(out, ".tmp-"+env+"-*")
	if err != nil {
		return fmt.Errorf("create temporary directory: %w", err)
	}
	defer os.RemoveAll(dir)

	for path, doc := range product {
		full := filepath.Join(dir, path)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			return fmt.Errorf("write product, previous tree left in place: %w", err)
		}
		if err := os.WriteFile(full, doc, 0o644); err != nil {
			return fmt.Errorf("write product, previous tree left in place: %w", err)
		}
	}

	targetDir := filepath.Join(out, env)
	backupDir := filepath.Join(out, ".old-"+env)
	if err := os.RemoveAll(backupDir); err != nil {
		return fmt.Errorf("remove stale backup, previous tree left in place: %w", err)
	}
	if _, err := os.Stat(targetDir); err == nil {
		if err := os.Rename(targetDir, backupDir); err != nil {
			return fmt.Errorf("move previous tree aside, previous tree left in place: %w", err)
		}
	}
	if err := os.Rename(dir, targetDir); err != nil {
		_ = os.Rename(backupDir, targetDir)
		return fmt.Errorf("install new tree, previous tree may be in %q: %w", backupDir, err)
	}
	_ = os.RemoveAll(backupDir)

	return nil
}
