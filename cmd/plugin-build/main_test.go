package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestValidatePluginKindsAndRepoFiles(t *testing.T) {
	root := t.TempDir()
	if err := validatePlugin(root, &pluginManifest{}); err == nil {
		t.Fatal("missing kind")
	}
	if err := validatePlugin(root, &pluginManifest{Kind: "service"}); err == nil {
		t.Fatal("unknown kind")
	}
	if err := validatePlugin(root, &pluginManifest{Kind: "resource-provider"}); err == nil {
		t.Fatal("resource-provider needs provider")
	}
	if err := validatePlugin(root, &pluginManifest{Kind: "resource-provider", Provider: json.RawMessage("null")}); err == nil {
		t.Fatal("null provider")
	}
	ok := &pluginManifest{
		Kind:     "resource-provider",
		Provider: json.RawMessage(`{"name":"redis","url":"http://redis-api.discoverd/clusters"}`),
	}
	if err := validatePlugin(root, ok); err != nil {
		t.Fatal(err)
	}
	if err := validatePlugin(root, &pluginManifest{Kind: "app"}); err != nil {
		t.Fatal(err)
	}

	ok.Hooks.Install = "hooks/install.sh"
	if err := validatePlugin(root, ok); err == nil {
		t.Fatal("missing hook file")
	}
	if err := os.MkdirAll(filepath.Join(root, "hooks"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "hooks", "install.sh"), []byte("#!/bin/sh\n"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := validatePlugin(root, ok); err != nil {
		t.Fatal(err)
	}

	ok.Build.Copy = map[string]string{"../secret": "/secret"}
	if err := validatePlugin(root, ok); err == nil || !strings.Contains(err.Error(), "inside the plugin repo") {
		t.Fatalf("path escape: %v", err)
	}
	ok.Build.Copy = map[string]string{"/etc/passwd": "/passwd"}
	if err := validatePlugin(root, ok); err == nil {
		t.Fatal("absolute copy")
	}
	ok.Build.Copy = map[string]string{"hooks": "/hooks"}
	if err := validatePlugin(root, ok); err == nil || !strings.Contains(err.Error(), "directory") {
		t.Fatalf("directory copy: %v", err)
	}
	ok.Build.Copy = map[string]string{"foo/../secret": "/secret"}
	if err := validatePlugin(root, ok); err == nil || !strings.Contains(err.Error(), "inside the plugin repo") {
		t.Fatalf("dot-dot copy: %v", err)
	}
}

func TestApplyBuildDefaults(t *testing.T) {
	root := t.TempDir()
	plugin := &pluginManifest{}
	applyBuildDefaults(root, plugin)
	if plugin.Build.Packages != "" {
		t.Fatalf("missing packages.sh: %s", plugin.Build.Packages)
	}
	if err := os.MkdirAll(filepath.Join(root, "img"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "img", "packages.sh"), []byte("#!/bin/sh\n"), 0755); err != nil {
		t.Fatal(err)
	}
	applyBuildDefaults(root, plugin)
	if plugin.Build.Packages != "img/packages.sh" {
		t.Fatalf("got %s", plugin.Build.Packages)
	}
	plugin.Build.Packages = "custom.sh"
	applyBuildDefaults(root, plugin)
	if plugin.Build.Packages != "custom.sh" {
		t.Fatal("must not overwrite an explicit packages path")
	}
}

func TestPublicURLs(t *testing.T) {
	layer, image, artifact := publicURLs("acme/plug", "v1", "dist", "manif")
	if layer != "https://github.com/acme/plug/releases/download/v1/{id}.squashfs" {
		t.Fatalf("layer=%s", layer)
	}
	if image != "https://github.com/acme/plug/releases/download/v1/manif.json" {
		t.Fatalf("image=%s", image)
	}
	if artifact != "https://github.com/acme/plug/releases/download/v1/image.json" {
		t.Fatalf("artifact=%s", artifact)
	}
	layer, _, _ = publicURLs("acme/plug", "dev", "/tmp/out", "manif")
	if !strings.HasPrefix(layer, "file://") || !strings.Contains(layer, "/layers/{id}.squashfs") {
		t.Fatalf("dev build must use file URLs: %s", layer)
	}
	layer, _, _ = publicURLs("", "v1", "/tmp/out", "manif")
	if !strings.HasPrefix(layer, "file://") {
		t.Fatalf("missing github repo: %s", layer)
	}
}

func TestWritePluginManifestAndLoad(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "flynn-plugin.json")
	raw := []byte(`{"name":"widget","kind":"app"}`)
	if err := os.WriteFile(src, raw, 0644); err != nil {
		t.Fatal(err)
	}
	got, data, err := loadPlugin(src)
	if err != nil || got.Name != "widget" || got.Kind != "app" || len(data) == 0 {
		t.Fatalf("%+v %v", got, err)
	}
	bad := filepath.Join(root, "bad.json")
	if err := os.WriteFile(bad, []byte("{"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := loadPlugin(bad); err == nil {
		t.Fatal("invalid plugin JSON must fail")
	}
	if err := writePluginManifest(filepath.Join(root, "out.json"), []byte("{"), "https://example/image.json", nil); err == nil {
		t.Fatal("invalid manifest JSON must not be rewritten")
	}

	out := filepath.Join(root, "dist", "flynn-plugin.json")
	if err := os.MkdirAll(filepath.Dir(out), 0755); err != nil {
		t.Fatal(err)
	}
	if err := writePluginManifest(out, raw, "https://example/image.json", &resolvedBase{
		Repo:    "acme/flynn",
		Version: "v1",
		Image:   "ubuntu-noble",
	}); err != nil {
		t.Fatal(err)
	}
	var obj map[string]interface{}
	b, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(b, &obj); err != nil {
		t.Fatal(err)
	}
	arts := obj["artifacts"].(map[string]interface{})
	if arts["image"] != "https://example/image.json" {
		t.Fatalf("artifacts=%v", arts)
	}
	build := obj["build"].(map[string]interface{})
	base := build["base"].(map[string]interface{})
	if base["repo"] != "acme/flynn" || base["image"] != "ubuntu-noble" {
		t.Fatalf("base=%v", base)
	}
}

func TestEnvOrAndFirstNonEmpty(t *testing.T) {
	t.Setenv("PLUGIN_BUILD_TEST_KEY", "")
	if envOr("PLUGIN_BUILD_TEST_KEY", "fallback") != "fallback" {
		t.Fatal("empty env")
	}
	t.Setenv("PLUGIN_BUILD_TEST_KEY", "set")
	if envOr("PLUGIN_BUILD_TEST_KEY", "fallback") != "set" {
		t.Fatal("set env")
	}
	if firstNonEmpty("", " ", "b") != " " {
		t.Fatalf("plugin-build firstNonEmpty keeps spaces: %q", firstNonEmpty("", " ", "b"))
	}
	if firstNonEmpty("", "", "b") != "b" || firstNonEmpty() != "" {
		t.Fatal("firstNonEmpty")
	}
}

func TestWriteCompactJSONDoesNotEscapeHTML(t *testing.T) {
	path := filepath.Join(t.TempDir(), "image.json")
	if err := writeCompactJSON(path, map[string]string{"sha": "abc<def>"}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	got := string(data)
	if !strings.Contains(got, "abc<def>") {
		t.Fatalf("layer hashes must not be HTML-escaped: %s", got)
	}
	if strings.Contains(got, `\u003c`) {
		t.Fatalf("unexpected unicode escape: %s", got)
	}
}

func TestCopyHookAssets(t *testing.T) {
	root := t.TempDir()
	out := filepath.Join(root, "dist")
	if err := os.MkdirAll(filepath.Join(root, "script"), 0755); err != nil {
		t.Fatal(err)
	}
	body := []byte("#!/bin/sh\nexit 0\n")
	if err := os.WriteFile(filepath.Join(root, "script", "install.sh"), body, 0755); err != nil {
		t.Fatal(err)
	}
	if err := copyHookAssets(root, out, pluginHooks{Install: "script/install.sh"}); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(out, "script-install.sh")
	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(body) {
		t.Fatalf("copied=%q", got)
	}
	if hookAssetName("script/install.sh") != "script-install.sh" {
		t.Fatal(hookAssetName("script/install.sh"))
	}
	if err := copyHookAssets(root, out, pluginHooks{Install: "../secret"}); err == nil {
		t.Fatal("path escape")
	}
}

func TestReleaseWorkflowUploadsHookScripts(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", ".github", "workflows", "release.yml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "*.sh") {
		t.Fatal("Build and Release must upload dist/*.sh hook assets")
	}
}
