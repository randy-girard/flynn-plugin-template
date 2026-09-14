package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	ct "github.com/flynn/flynn/controller/types"
)

func TestPickBaseLayersPrefersUbuntuNoble(t *testing.T) {
	noble := layer("noble", 10)
	other := layer("other", 20)
	images := map[string]*ct.Artifact{
		"ubuntu-noble": artifact(noble, other),
		"blobstore":    artifact(layer("blob-os", 11), layer("blob-app", 12)),
	}
	got, err := pickBaseLayers(images, "blobstore")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].ID != "noble" || got[1].ID != "other" {
		t.Fatalf("got %#v, want ubuntu-noble's layers", ids(got))
	}
}

func TestPickBaseLayersUsesFirstLayerOfDonor(t *testing.T) {
	osLayer := layer("os", 319729664)
	pkg := layer("pkg", 100)
	bin := layer("bin", 200)
	images := map[string]*ct.Artifact{
		"blobstore": artifact(osLayer, pkg, bin),
		"redis":     artifact(osLayer, layer("redis-pkg", 3)),
	}
	got, err := pickBaseLayers(images, "blobstore")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ID != "os" || got[0].Length != 319729664 {
		t.Fatalf("got %#v, want first blobstore layer (ubuntu-noble)", ids(got))
	}
}

func TestPickBaseLayersMissing(t *testing.T) {
	_, err := pickBaseLayers(map[string]*ct.Artifact{"redis": artifact(layer("os", 1))}, "nope")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestVerifyLayerFileAcceptsFlynnBuildID(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/layer.squashfs"
	if err := os.WriteFile(path, []byte("squashfs-bytes"), 0644); err != nil {
		t.Fatal(err)
	}
	got, err := hashLayer(path)
	if err != nil {
		t.Fatal(err)
	}
	layer := &ct.ImageLayer{
		ID:     "flynn-build-input-id",
		Length: got.Length,
		Hashes: map[string]string{"sha512_256": got.Hashes["sha512_256"]},
	}
	if err := verifyLayerFile(path, layer); err != nil {
		t.Fatal(err)
	}
}

func TestLatestPublishedRelease(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/randy-girard/flynn/releases/latest" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(githubRelease{TagName: "v20260911.0"})
	}))
	defer srv.Close()
	origBase, origHTTP := githubAPIBase, githubHTTP
	githubAPIBase, githubHTTP = srv.URL, srv.Client()
	defer func() {
		githubAPIBase, githubHTTP = origBase, origHTTP
	}()

	tag, err := latestPublishedRelease("randy-girard/flynn")
	if err != nil {
		t.Fatal(err)
	}
	if tag != "v20260911.0" {
		t.Fatalf("tag %q", tag)
	}
}

func layer(id string, n int64) *ct.ImageLayer {
	return &ct.ImageLayer{
		ID:     id,
		Type:   ct.ImageLayerTypeSquashfs,
		Length: n,
		Hashes: map[string]string{"sha512_256": id},
	}
}

func artifact(layers ...*ct.ImageLayer) *ct.Artifact {
	man := &ct.ImageManifest{
		Type: ct.ImageManifestTypeV1,
		Rootfs: []*ct.ImageRootfs{{
			Platform: ct.DefaultImagePlatform,
			Layers:   layers,
		}},
	}
	return &ct.Artifact{
		Type:        ct.ArtifactTypeFlynn,
		RawManifest: man.RawManifest(),
	}
}

func ids(layers []*ct.ImageLayer) []string {
	out := make([]string, len(layers))
	for i, l := range layers {
		out[i] = l.ID
	}
	return out
}
