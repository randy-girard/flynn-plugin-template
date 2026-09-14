package main

import (
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	ct "github.com/flynn/flynn/controller/types"
)

const (
	defaultFlynnRepo  = "randy-girard/flynn"
	defaultBaseImage  = "blobstore"
	ubuntuNobleImage  = "ubuntu-noble"
	githubAPIDefault  = "https://api.github.com"
	githubDownloadFmt = "https://github.com/%s/releases/download/%s/%s.squashfs"
)

var (
	githubAPIBase = githubAPIDefault
	githubHTTP    = &http.Client{Timeout: 60 * time.Second}
)

type pluginBase struct {
	Repo    string `json:"repo,omitempty"`
	Version string `json:"version,omitempty"`
	Image   string `json:"image,omitempty"`
}

type resolvedBase struct {
	Repo    string
	Version string
	Image   string
	Layers  []*ct.ImageLayer
	Files   []string
}

type githubRelease struct {
	TagName    string `json:"tag_name"`
	Draft      bool   `json:"draft"`
	Prerelease bool   `json:"prerelease"`
}

func resolveFlynnBase(repo, version, image, cacheDir string) (*resolvedBase, error) {
	if repo == "" {
		repo = defaultFlynnRepo
	}
	if image == "" {
		image = defaultBaseImage
	}
	if version == "" || version == "latest" {
		tag, err := latestPublishedRelease(repo)
		if err != nil {
			return nil, err
		}
		version = tag
	}

	fmt.Fprintf(os.Stderr, "==> Flynn base %s@%s (image %s)\n", repo, version, image)

	images, err := loadFlynnImages(repo, version, cacheDir)
	if err != nil {
		return nil, err
	}
	layers, err := pickBaseLayers(images, image)
	if err != nil {
		return nil, err
	}

	files := make([]string, 0, len(layers))
	for _, layer := range layers {
		path, err := fetchFlynnLayer(repo, version, layer, cacheDir)
		if err != nil {
			return nil, err
		}
		files = append(files, path)
	}
	return &resolvedBase{
		Repo:    repo,
		Version: version,
		Image:   image,
		Layers:  layers,
		Files:   files,
	}, nil
}

func pickBaseLayers(images map[string]*ct.Artifact, image string) ([]*ct.ImageLayer, error) {
	if art, ok := images[ubuntuNobleImage]; ok {
		layers := artifactSquashfsLayers(art)
		if len(layers) == 0 {
			return nil, fmt.Errorf("Flynn images.json %q has no squashfs layers", ubuntuNobleImage)
		}
		return cloneLayers(layers), nil
	}
	art, ok := images[image]
	if !ok {
		names := make([]string, 0, len(images))
		for k := range images {
			names = append(names, k)
		}
		return nil, fmt.Errorf("Flynn images.json has no %q (and no %q); have %s", image, ubuntuNobleImage, strings.Join(names, ", "))
	}
	layers := artifactSquashfsLayers(art)
	if len(layers) == 0 {
		return nil, fmt.Errorf("Flynn image %q has no squashfs layers", image)
	}
	// First layer of blobstore/redis/postgres/… is the shared ubuntu-noble OS.
	return cloneLayers(layers[:1]), nil
}

func artifactSquashfsLayers(art *ct.Artifact) []*ct.ImageLayer {
	if art == nil {
		return nil
	}
	man := art.Manifest()
	if man == nil {
		return nil
	}
	var out []*ct.ImageLayer
	for _, rf := range man.Rootfs {
		if rf == nil {
			continue
		}
		for _, l := range rf.Layers {
			if l != nil {
				out = append(out, l)
			}
		}
	}
	return out
}

func cloneLayers(in []*ct.ImageLayer) []*ct.ImageLayer {
	out := make([]*ct.ImageLayer, len(in))
	for i, l := range in {
		cp := *l
		if l.Hashes != nil {
			cp.Hashes = make(map[string]string, len(l.Hashes))
			for k, v := range l.Hashes {
				cp.Hashes[k] = v
			}
		}
		if l.Meta != nil {
			cp.Meta = make(map[string]string, len(l.Meta))
			for k, v := range l.Meta {
				cp.Meta[k] = v
			}
		}
		out[i] = &cp
	}
	return out
}

func latestPublishedRelease(repo string) (string, error) {
	url := githubAPIBase + "/repos/" + repo + "/releases/latest"
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "flynn-plugin-build")
	req.Header.Set("Accept", "application/vnd.github+json")
	if tok := os.Getenv("GITHUB_TOKEN"); tok != "" {
		req.Header.Set("Authorization", "Bearer "+tok)
	}
	res, err := githubHTTP.Do(req)
	if err != nil {
		return "", fmt.Errorf("GitHub latest release for %s: %w", repo, err)
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if res.StatusCode != http.StatusOK {
		return "", fmt.Errorf("GitHub latest release for %s: HTTP %d: %s", repo, res.StatusCode, strings.TrimSpace(string(body)))
	}
	var rel githubRelease
	if err := json.Unmarshal(body, &rel); err != nil {
		return "", fmt.Errorf("parse GitHub latest release: %w", err)
	}
	if rel.TagName == "" {
		return "", fmt.Errorf("GitHub latest release for %s has no tag_name", repo)
	}
	return rel.TagName, nil
}

func loadFlynnImages(repo, version, cacheDir string) (map[string]*ct.Artifact, error) {
	dir := filepath.Join(cacheDir, "flynn", version)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, err
	}
	gzPath := filepath.Join(dir, "images.json.gz")
	url := fmt.Sprintf("https://github.com/%s/releases/download/%s/images.json.gz", repo, version)
	if _, err := os.Stat(gzPath); err != nil {
		fmt.Fprintf(os.Stderr, "downloading %s\n", url)
		if err := curl(gzPath+".tmp", url); err != nil {
			return nil, err
		}
		if err := os.Rename(gzPath+".tmp", gzPath); err != nil {
			return nil, err
		}
	}
	f, err := os.Open(gzPath)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return nil, fmt.Errorf("images.json.gz: %w", err)
	}
	defer gz.Close()
	var images map[string]*ct.Artifact
	if err := json.NewDecoder(gz).Decode(&images); err != nil {
		return nil, fmt.Errorf("parse images.json: %w", err)
	}
	return images, nil
}

func fetchFlynnLayer(repo, version string, layer *ct.ImageLayer, cacheDir string) (string, error) {
	if layer == nil || layer.ID == "" {
		return "", fmt.Errorf("Flynn base layer is missing an id")
	}
	dir := filepath.Join(cacheDir, "flynn", version, "layers")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}
	path := filepath.Join(dir, layer.ID+".squashfs")
	if st, err := os.Stat(path); err == nil {
		if layer.Length == 0 || st.Size() == layer.Length {
			if err := verifyLayerFile(path, layer); err == nil {
				return path, nil
			}
			_ = os.Remove(path)
		} else {
			_ = os.Remove(path)
		}
	}
	url := fmt.Sprintf(githubDownloadFmt, repo, version, layer.ID)
	fmt.Fprintf(os.Stderr, "downloading %s (%d bytes)\n", url, layer.Length)
	tmp := path + ".tmp"
	if err := curl(tmp, url); err != nil {
		return "", err
	}
	if err := os.Rename(tmp, path); err != nil {
		return "", err
	}
	if err := verifyLayerFile(path, layer); err != nil {
		_ = os.Remove(path)
		return "", err
	}
	return path, nil
}

func verifyLayerFile(path string, layer *ct.ImageLayer) error {
	got, err := hashLayer(path)
	if err != nil {
		return err
	}
	if layer.Length > 0 && got.Length != layer.Length {
		return fmt.Errorf("layer %s size %d, want %d", layer.ID, got.Length, layer.Length)
	}
	want := layer.Hashes["sha512_256"]
	if want == "" {
		return fmt.Errorf("layer %s missing sha512_256", layer.ID)
	}
	if got.Hashes["sha512_256"] != want {
		return fmt.Errorf("layer %s sha512_256 %s, want %s", layer.ID, got.Hashes["sha512_256"], want)
	}
	return nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}
