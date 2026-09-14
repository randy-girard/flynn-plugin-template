package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

func installBinaries(repo, rootfs string, plugin *pluginManifest) error {
	binDir, err := os.MkdirTemp("", "flynn-plugin-bin-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(binDir)

	for pkg, dest := range plugin.Build.Go {
		if dest == "" {
			return fmt.Errorf("build.go[%q]: empty destination", pkg)
		}
		out := filepath.Join(binDir, filepath.Base(dest))
		fmt.Fprintf(os.Stderr, "go build %s -> %s\n", pkg, dest)
		cmd := exec.Command("go", "build", "-trimpath", "-ldflags=-s -w", "-o", out, pkg)
		cmd.Dir = repo
		cmd.Stdout = os.Stderr
		cmd.Stderr = os.Stderr
		cmd.Env = append(os.Environ(),
			"CGO_ENABLED=0",
			"GOOS=linux",
			"GOARCH=amd64",
			"GOFLAGS=-mod=mod -buildvcs=false",
		)
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("go build %s: %w", pkg, err)
		}
		if err := installFile(out, filepath.Join(rootfs, dest), 0755); err != nil {
			return err
		}
	}

	for src, dest := range plugin.Build.Copy {
		if dest == "" {
			return fmt.Errorf("build.copy[%q]: empty destination", src)
		}
		if err := installFile(filepath.Join(repo, src), filepath.Join(rootfs, dest), 0755); err != nil {
			return err
		}
	}
	return nil
}

func installFile(src, dest string, mode os.FileMode) error {
	if err := sudoCommand("mkdir", "-p", filepath.Dir(dest)).Run(); err != nil {
		return err
	}
	if err := sudoCommand("install", "-m", fmt.Sprintf("%04o", mode), src, dest).Run(); err != nil {
		return fmt.Errorf("install %s -> %s: %w", src, dest, err)
	}
	return nil
}
