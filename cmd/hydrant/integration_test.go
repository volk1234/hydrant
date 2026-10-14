package main

import (
	"bytes"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestGoldenRenderAgainstTestdata(t *testing.T) {
	outDir := t.TempDir()

	cmd := exec.Command("go", "run", ".", "-repo", "../../testdata", "-env", "dev", "-out", outDir)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("hydrant run failed: %v\nstderr: %s", err, stderr.String())
	}

	wantSummary := "1 legacy apps skipped"
	if !bytes.Contains(stderr.Bytes(), []byte(wantSummary)) {
		t.Errorf("stderr does not contain %q; got:\n%s", wantSummary, stderr.String())
	}

	expectedDir := filepath.Join("..", "..", "testdata-rendered", "dev")
	actualDir := filepath.Join(outDir, "dev")

	// Verify all expected files exist and match exactly
	err := filepath.WalkDir(expectedDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}

		rel, err := filepath.Rel(expectedDir, path)
		if err != nil {
			return err
		}

		expectedContent, err := os.ReadFile(path)
		if err != nil {
			return err
		}

		actualPath := filepath.Join(actualDir, rel)
		actualContent, err := os.ReadFile(actualPath)
		if err != nil {
			t.Errorf("missing rendered file %s: %v", rel, err)
			return nil
		}

		if !bytes.Equal(expectedContent, actualContent) {
			t.Errorf("content mismatch in %s:\n--- expected ---\n%s\n--- actual ---\n%s", rel, string(expectedContent), string(actualContent))
		}

		return nil
	})
	if err != nil {
		t.Fatalf("failed walking expectedDir: %v", err)
	}

	// Verify there are no unexpected extra files in actualDir
	err = filepath.WalkDir(actualDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}

		rel, err := filepath.Rel(actualDir, path)
		if err != nil {
			return err
		}

		expectedPath := filepath.Join(expectedDir, rel)
		if _, err := os.Stat(expectedPath); os.IsNotExist(err) {
			t.Errorf("unexpected extra file rendered: %s", rel)
		}

		return nil
	})
	if err != nil {
		t.Fatalf("failed walking actualDir: %v", err)
	}
}

func TestPositionalArgumentRejected(t *testing.T) {
	cmd := exec.Command("go", "run", ".", "-repo", "../../testdata", "-env", "dev", "-out", t.TempDir(), "unexpected-arg")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err == nil {
		t.Fatal("expected error with positional argument, got success")
	}
	wantMsg := `argument "unexpected-arg": hydrant takes no positional arguments`
	if !bytes.Contains(stderr.Bytes(), []byte(wantMsg)) {
		t.Errorf("stderr = %q, want it to contain %q", stderr.String(), wantMsg)
	}
}

func TestVersionFlag(t *testing.T) {
	cmd := exec.Command("go", "run", ".", "-version")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("hydrant -version failed: %v\nstderr: %s", err, stderr.String())
	}
	want := "hydrant dev\n"
	if stdout.String() != want {
		t.Errorf("stdout = %q, want %q", stdout.String(), want)
	}
}


