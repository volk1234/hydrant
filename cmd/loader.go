package main

import (
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
)

func loadConfigs(repo, env string) ([]ConfigFile, []error, error) {
	root := filepath.Join(repo, appsDir)
	file := "config-" + env + ".yaml"
	var configs []ConfigFile
	var cfgErrs []error

	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || d.Name() != file {
			return nil
		}
		cf, errs := loadOneConfig(repo, p)
		if len(errs) > 0 {
			cfgErrs = append(cfgErrs, errs...)
			return nil
		}
		configs = append(configs, cf)
		return nil
	})

	return configs, cfgErrs, err
}

func loadOneConfig(repo, fullPath string) (ConfigFile, []error) {
	relPath, err := filepath.Rel(repo, fullPath)
	if err != nil {
		return ConfigFile{}, []error{fmt.Errorf("%s: cannot compute path relative to repository root: %w", fullPath, err)}
	}
	relPath = filepath.ToSlash(relPath)

	data, err := os.ReadFile(fullPath)
	if err != nil {
		return ConfigFile{}, []error{fmt.Errorf("%s: cannot read file: %w", relPath, err)}
	}
	cfg, err := parseConfig(data, relPath)
	if err != nil {
		return ConfigFile{}, []error{err}
	}
	dir := path.Dir(relPath)
	errs := validateConfig(cfg, relPath)
	if path.Base(dir) != cfg.App.Name {
		errs = append(errs, fmt.Errorf("%s: app.name %q does not match directory %q", relPath, cfg.App.Name, path.Base(dir)))
	}
	if len(errs) > 0 {
		return ConfigFile{}, errs
	}

	return ConfigFile{Config: cfg, RelPath: relPath, Dir: dir}, nil
}

func findStrayConfigs(repo string, envs []string) ([]string, error) {
	root := filepath.Join(repo, appsDir)
	var stray []string

	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !strings.HasPrefix(d.Name(), "config-") || !strings.HasSuffix(d.Name(), ".yaml") {
			return nil
		}
		name := strings.TrimSuffix(strings.TrimPrefix(d.Name(), "config-"), ".yaml")
		if !slices.Contains(envs, name) {
			relPath, err := filepath.Rel(repo, p)
			if err != nil {
				return fmt.Errorf("%s: cannot compute path relative to repository root: %w", repo, err)
			}
			stray = append(stray, fmt.Sprintf("%s: environment %q has no cluster directory; file is ignored", filepath.ToSlash(relPath), name))

		}
		return nil
	})
	return stray, err
}
