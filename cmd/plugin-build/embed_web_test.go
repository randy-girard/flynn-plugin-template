package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestEmbedWebSkipsUnchangedDist(t *testing.T) {
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	script := filepath.Join(root, "script", "lib", "embed-web.sh")
	dir := t.TempDir()
	web := filepath.Join(dir, "web")
	if err := os.MkdirAll(filepath.Join(web, "src"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(web, "dist"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(web, "package.json"), []byte(`{"name":"ui"}`+"\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(web, "src", "app.js"), []byte("console.log(1)\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(web, "dist", "index.html"), []byte("built\n"), 0644); err != nil {
		t.Fatal(err)
	}
	cache := filepath.Join(dir, "cache")
	fingerprint := exec.Command("bash", "-c", `source "$1" && plugin_web_fingerprint "$2"`, "bash", script, web)
	fpOut, err := fingerprint.Output()
	if err != nil {
		t.Fatalf("fingerprint: %v %s", err, fpOut)
	}
	if err := os.MkdirAll(cache, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cache, "web.fingerprint"), fpOut, 0644); err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command("bash", "-c", `source "$1" && plugin_embed_web "$2"`, "bash", script, dir)
	cmd.Env = append(os.Environ(), "FLYNN_PLUGIN_BUILD_CACHE="+cache)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("embed: %v\n%s", err, out)
	}
	if !bytes.Contains(out, []byte("web/ unchanged")) {
		t.Fatalf("expected skip, got %s", out)
	}
}

func TestPluginNpmPlatform(t *testing.T) {
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	script := filepath.Join(root, "script", "lib", "embed-web.sh")
	out, err := exec.Command("bash", "-c", `source "$1" && plugin_npm_platform`, "bash", script).Output()
	if err != nil {
		t.Fatalf("platform: %v %s", err, out)
	}
	got := strings.TrimSpace(string(out))
	if got == "" {
		t.Fatal("plugin_npm_platform was empty")
	}
	if !strings.Contains(got, "-") {
		t.Fatalf("expected os-arch token, got %q", got)
	}
}

func TestEmbedWebReinstallsNpmWhenRollupNativeMissing(t *testing.T) {
	root, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	script := filepath.Join(root, "script", "lib", "embed-web.sh")
	dir := t.TempDir()
	web := filepath.Join(dir, "web")
	if err := os.MkdirAll(filepath.Join(web, "src"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(web, "node_modules", "rollup", "dist"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(web, "package.json"), []byte(`{"name":"ui"}`+"\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(web, "package-lock.json"), []byte(`{"lockfileVersion":3}`+"\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(web, "src", "app.js"), []byte("console.log(1)\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(web, "node_modules", "rollup", "package.json"), []byte(`{"version":"4.60.4"}`+"\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(web, "node_modules", "rollup", "dist", "native.js"), []byte("throw new Error('missing native')\n"), 0644); err != nil {
		t.Fatal(err)
	}

	platformOut, err := exec.Command("bash", "-c", `source "$1" && plugin_npm_platform`, "bash", script).Output()
	if err != nil {
		t.Fatalf("platform: %v %s", err, platformOut)
	}
	lockHash, err := exec.Command("bash", "-c", `source "$1" && plugin_file_hash "$2"`, "bash", script, filepath.Join(web, "package-lock.json")).Output()
	if err != nil {
		t.Fatalf("lockhash: %v %s", err, lockHash)
	}
	cache := filepath.Join(dir, "cache")
	if err := os.MkdirAll(cache, 0755); err != nil {
		t.Fatal(err)
	}
	stamp := strings.TrimSpace(string(lockHash)) + " " + strings.TrimSpace(string(platformOut)) + "\n"
	if err := os.WriteFile(filepath.Join(cache, "web.lockhash"), []byte(stamp), 0644); err != nil {
		t.Fatal(err)
	}

	bin := filepath.Join(dir, "bin")
	if err := os.MkdirAll(bin, 0755); err != nil {
		t.Fatal(err)
	}
	logPath := filepath.Join(dir, "npm.log")
	npm := `#!/usr/bin/env bash
echo "$*" >> "$NPM_LOG"
if [[ "$1" == "ci" ]]; then
  mkdir -p node_modules/rollup/dist
  printf '%s\n' '{"version":"4.60.4"}' > node_modules/rollup/package.json
  printf '%s\n' 'module.exports = {};' > node_modules/rollup/dist/native.js
fi
if [[ "$1" == "run" ]]; then
  mkdir -p dist
  echo built > dist/index.html
fi
if [[ "$1" == "install" ]]; then
  mkdir -p node_modules/rollup/dist
  printf '%s\n' 'module.exports = {};' > node_modules/rollup/dist/native.js
fi
`
	if err := os.WriteFile(filepath.Join(bin, "npm"), []byte(npm), 0755); err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command("bash", "-c", `source "$1" && plugin_embed_web "$2"`, "bash", script, dir)
	cmd.Env = append(os.Environ(),
		"FLYNN_PLUGIN_BUILD_CACHE="+cache,
		"NPM_LOG="+logPath,
		"PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"),
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("embed: %v\n%s", err, out)
	}
	if !bytes.Contains(out, []byte("npm ci --include=optional")) {
		t.Fatalf("expected npm ci despite matching lock stamp, got %s", out)
	}
	logged, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(logged, []byte("ci --include=optional")) {
		t.Fatalf("fake npm did not run ci, log:\n%s", logged)
	}
}
