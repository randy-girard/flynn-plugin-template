# Flynn plugin template

Skeleton repository for a Flynn plugin. Copy this layout for a new first-party
cluster app: a resource provider (Redis, MariaDB, MongoDB, Kafka, ClickHouse)
or something that is not a database, such as a dashboard.

A plugin is a **git repo** with `flynn-plugin.json` at the root — not a user
`git push` app. `flynn-host plugin install` (cluster operator, not the user
`flynn` CLI) reads that manifest, pulls a **prebuilt Flynn Artifact** from GitHub
Releases, runs an optional **install hook** against the cluster, and deploys a
system app. Resource-provider plugins also register a provider; `app` plugins
do not.

## Plugin kinds

| `kind` | Use for | On install |
|--------|---------|------------|
| `app` | Dashboard, UI, other first-party cluster software | Deploy the system app. No `add-provider`. |
| `resource-provider` | Redis, MariaDB, and other datastores | Deploy the system app and register `provider`. |

This template is an `app` (a small HTTP service). Database plugins add
`provider`, set `flynn-datastore` on the app, and usually omit install hooks.

## Layout

```text
flynn-plugin.json     Install contract (name, kind, app spec, optional provider/cli/hooks)
img/packages.sh       apt packages over Flynn's ubuntu-noble layer
cmd/<name>/           Plugin process
cmd/plugin-build/     Writes dist/image.json + squashfs layers Flynn pulls
script/plugin-build   Wrapper: CGO_ENABLED=0 go run ./cmd/plugin-build
script/install.sh     Optional hook: cluster setup before the app is deployed
.github/workflows/    CI (unit tests) and manual Build and Release
```

## Install hooks

If the plugin needs cluster work **before** its app starts (create a route,
seed controller data, wait on postgres, register OAuth), set:

```json
"hooks": {
  "install": "script/install.sh",
  "upgrade": "script/upgrade.sh",
  "uninstall": "script/uninstall.sh"
}
```

`flynn-host plugin install` runs `hooks.install` on a cluster node with
cluster-admin credentials (`CONTROLLER_KEY`, `CLUSTER_DOMAIN`,
`FLYNN_PLUGIN_NAME`, `FLYNN_PLUGIN_KIND`, `FLYNN_PLUGIN_ROOT`). Working
directory is the plugin checkout. A non-zero exit aborts install.

Hooks are optional. Redis does not need one; a dashboard often does. Scripts
must be idempotent: `plugin update` runs `hooks.upgrade` if set, otherwise
`hooks.install` again.

## What Flynn pulls

`script/plugin-build` produces Flynn's Artifact format (`type: flynn`):

| File | Role |
|------|------|
| `dist/image.json` | Controller Artifact: `manifest`, `hashes` (`sha512_256`), `layer_url_template` |
| `dist/<manifest-id>.json` | ImageManifest bytes that `artifact.uri` points at |
| `dist/layers/<layer-id>.squashfs` | Flynn ubuntu-noble (same id as Flynn) plus the plugin delta |
| `dist/flynn-plugin.json` | Manifest with `artifacts.image` set to the Artifact URL |

Layer URLs are **HTTPS GitHub Release assets**, not `file://` host-cache paths
(those are only for images shipped inside the Flynn tarball):

```text
https://github.com/<owner>/<repo>/releases/download/<tag>/{id}.squashfs
```

`{id}` is the layer ID (sha512_256 of the squashfs file). Hosts expand
`layer_url_template` the same way they do for other Flynn images.

When `-github-repo` / `GITHUB_REPOSITORY` and a release `-version` (git tag) are
set, those HTTPS URLs are written into `image.json`. Local builds use `file://`
paths under `dist/`.

## Build locally

Image builds are Linux/amd64. `plugin-build` pulls Flynn's **ubuntu-noble** squashfs
from a Flynn GitHub Release (`images.json.gz`; the first layer of `blobstore` /
`redis` / `postgres` is that OS), overlays `img/packages.sh` + gobuild, and
squashfs only the delta.

Default Flynn source is `randy-girard/flynn` **latest published** tag. Pin a
release for reproducible plugin images:

```bash
./script/plugin-build -flynn-version v20260911.0
# or in flynn-plugin.json build.base.version, or FLYNN_VERSION=
```

On **Linux**, `./script/plugin-build` runs that natively (`squashfs-tools` + sudo + overlayfs).

On **macOS**, the same commands use Docker Desktop (linux/amd64, privileged),
the same split as Flynn `script/run-unit-tests`. That is only a Linux userland
for overlay/squashfs and `go test` — not Flynn’s ZFS/Vagrant cluster path.

```bash
./script/run-unit-tests
./script/plugin-build
./script/plugin-build -skip-image
make test-unit
make plugin-build
```

`PLUGIN_BUILD_DOCKER=0` forces native (fails on macOS without Linux deps).
`PLUGIN_BUILD_DOCKER=1` forces Docker even on Linux.

## GitHub Actions

- **CI** (`ci.yml`): `gofmt` and `go test` on push/PR (same split as Flynn unit tests).
- **Build and Release** (`release.yml`): **manual only** (`workflow_dispatch`). Enter a version
  like `v20260914.0`. Default is a GitHub draft/prerelease, matching Flynn. The
  workflow builds squashfs layers, then creates the GitHub Release with
  `image.json`, `<manifest-id>.json`, and `{id}.squashfs`.

`flynn-host plugin install` pulls `artifacts.image` from the release (stable
name `image.json`). Production plugins publish those layers.

## flynn-plugin.json

See the example in this repo (`kind: app`). Resource-provider plugins also set:

- `provider.name` / `provider.url` — controller `add-provider` (discoverd URL)
- `image_env` — e.g. `{ "REDIS_IMAGE_ID": "self" }` so child jobs use this image

Common fields:

- `kind` — `app` or `resource-provider`
- `app` — system app spec (`flynn-system-app`, `flynn-plugin` meta)
- `inject_env` — cluster secrets the installer copies in (`CONTROLLER_KEY`, …)
- `cli` — optional catalog entry the user `flynn` CLI downloads from the cluster
- `hooks.install` — optional script run against the cluster before deploy
- `build.entrypoint` — Flynn ImageManifest args
- `build.base` — Flynn GitHub repo/tag/`images.json` key for ubuntu-noble (`version: latest` or a pin like `v20260911.0`)
- `build.packages` — chroot apt overlay on that base
- `build.go` / `build.copy` — binaries and scripts installed into the delta layer
- `artifacts.image` — filled by `plugin-build` / the release workflow

Pin Flynn with a Go module `replace` to the Flynn revision you build against:

```go
replace github.com/flynn/flynn => github.com/randy-girard/flynn <commit>
```

Do not vendor `pkg/sirenia`, discoverd, or the controller client; `require`
`github.com/flynn/flynn` instead.

## Install

```text
flynn-host plugin install example
flynn-host plugin install /path/to/this-repo
flynn-host plugin install https://github.com/OWNER/flynn-plugin-example.git --ref v20260914.0
```

The user `flynn` CLI never installs plugins. After install it shows commands
listed in that cluster's CLI catalog (`cli` is optional; an `app` plugin may
have none).
