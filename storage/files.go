package storage

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

type RunPaths struct {
	RunID    string
	Original string
	Enriched string
	Details  string
	Manifest string
}

func PrepareRun(outputDir, inputPath string) (RunPaths, error) {
	runID := time.Now().UTC().Format("20060102_150405")
	runDir := filepath.Join(outputDir, runID)
	if err := os.MkdirAll(runDir, 0755); err != nil {
		return RunPaths{}, err
	}
	paths := RunPaths{
		RunID:    runID,
		Original: filepath.Join(runDir, "original_inventory.csv"),
		Enriched: filepath.Join(runDir, "enriched_inventory.csv"),
		Details:  filepath.Join(runDir, "vehicle_details.csv"),
		Manifest: filepath.Join(runDir, "manifest.json"),
	}
	if err := CopyFile(inputPath, paths.Original); err != nil {
		return RunPaths{}, fmt.Errorf("copy original: %w", err)
	}
	return paths, nil
}

func CopyFile(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	if err != nil {
		return err
	}
	return out.Sync()
}
