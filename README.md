# Flynn plugin template

[![coverage](.github/badges/coverage.svg)](https://github.com/randy-girard/flynn-plugin-template/actions/workflows/ci.yml)

Skeleton repository for a Flynn plugin. Copy this layout for a new first-party
cluster app: a resource provider (Redis, MariaDB, MongoDB, Kafka, ClickHouse)
or something that is not a database, such as a dashboard.

A plugin is a **git repo** with `flynn-plugin.json` at the root — not a user
`git push` app. `flynn-host plugin:install` (cluster operator, not the user
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
script/uninstall.sh   Optional hook: cluster cleanup before the app is deleted
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

`flynn-host plugin:install` runs `hooks.install` on a cluster node with
cluster-admin credentials (`CONTROLLER_KEY`, `CLUSTER_DOMAIN`,
`FLYNN_PLUGIN_NAME`, `FLYNN_PLUGIN_KIND`, `FLYNN_PLUGIN_ROOT`). Working
directory is the plugin checkout. A non-zero exit aborts install.

Hooks are optional. Redis does not need one; a dashboard often does. Scripts
must be idempotent: `plugin:update` runs `hooks.upgrade` if set, otherwise
`hooks.install` again.

`flynn-host plugin:uninstall` runs `hooks.uninstall` (if set) before deleting
the plugin app. A non-zero exit aborts uninstall. Resource-provider plugins
with leftover provisioned resources refuse unless `--force`.

## What Flynn pulls

`script/plugin-build` produces Flynn's Artifact format (`type: flynn`):

| File | Role |
|------|------|
| `dist/image.json` | Controller Artifact: `manifest`, `hashes` (`sha512_256`), `layer_url_template` |
| `dist/<manifest-id>.json` | ImageManifest bytes that `artifact.uri` points at |
| `dist/layers/<layer-id>.squashfs` | Flynn ubuntu-noble (local overlay only) plus the plugin delta |
| `dist/flynn-plugin.json` | Manifest with `artifacts.image` set to the Artifact URL |

The GitHub Release publishes the **plugin delta** squashfs next to `image.json`.
Flynn ubuntu-noble stays on the Flynn GitHub Release (same layer id); hosts
fetch it from artifact meta `flynn.plugin.base`. Re-uploading that OS layer from
every plugin (~200MiB × N jobs) 502s `uploads.github.com`.

```text
https://github.com/<owner>/<plugin-repo>/releases/download/<tag>/{delta-id}.squashfs
https://github.com/<owner>/flynn/releases/download/<flynn-tag>/{ubuntu-noble-id}.squashfs
```

`{id}` is the layer ID (sha512_256 of the squashfs file).

When `-github-repo` / `GITHUB_REPOSITORY` and a release `-version` (git tag) are
set, those HTTPS URLs are written into `image.json`. Local builds use `file://`
paths under `dist/`.

## Build locally

Image builds are Linux/amd64. `plugin-build` pulls Flynn's **ubuntu-noble** squashfs
from a Flynn GitHub Release (`images.json.gz`). Named `ubuntu-noble` is often
omitted from that file; plugin-build then takes layer 0 of `postgres` /
`gitreceive` / similar (not `blobstore`, which is busybox after image-slim).
It overlays `img/packages.sh` + gobuild and squashfs only the delta.

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

Unit tests write HTML coverage under `coverage/` (gitignored): open `coverage/index.html`. Set `PLUGIN_SKIP_COVERAGE=1` to skip the report.

`PLUGIN_BUILD_DOCKER=0` forces native (fails on macOS without Linux deps).
`PLUGIN_BUILD_DOCKER=1` forces Docker even on Linux.

## GitHub Actions

- **CI** (`ci.yml`): `gofmt`, release-note checks, and `go test` on push/PR (same split as Flynn unit tests).
- **Build and Release** (`release.yml`): **manual only** (`workflow_dispatch`). Enter a version
  like `v20260914.0.0` (`vYYYYMMDD.N.P`). Leave empty to pick the next tag; plugin-only rebuilds increment the last number while Flynn stays `vYYYYMMDD.N`. Default is a published GitHub Release (not draft, not
  prerelease). Notes group conventional commits the same way Flynn does, with a
  Full Changelog compare link and install commands. The workflow builds squashfs
  layers, then creates the GitHub Release with `image.json`, `<manifest-id>.json`,
  hook scripts, and the plugin **delta** squashfs (not Flynn ubuntu-noble).

`flynn-host plugin:install` pulls `artifacts.image` from the release (stable
name `image.json`). Production plugins publish those layers.

## flynn-plugin.json

See the example in this repo (`kind: app`). Resource-provider plugins also set:

- `provider.name` / `provider.url` — controller `add-provider` (discoverd URL)
- `image_env` — e.g. `{ "REDIS_IMAGE_ID": "self" }` so child jobs use this image

Common fields:

- `kind` — `app` or `resource-provider`
- `app` — system app spec (`flynn-system-app`, `flynn-plugin` meta)
- `inject_env` — cluster secrets the installer copies in (`CONTROLLER_KEY`, …)
- `cli` — optional user `flynn` command. After install the laptop fetches this
  from the cluster (`command`, `usage`, `doc`, `actions`). The CLI does not
  compile plugin handlers; runnable plugins set `doc` (docopt) and `actions`
  (cluster jobs using the plugin/resource image). See Flynn `docs/content/plugins.md`.
  This template declares `cli.subcommands: ["ping"]` as a placeholder: there is
  no `doc`/`actions` spec, so `flynn example` does not appear on `flynn help`
  until you add those fields (or omit `cli` entirely for an HTTP system app).
- `setup` — optional TTY prompts (`env`, `prompt`, `secret`, `optional`, `generate`)
- `resources` — existing providers to attach on first install (for example `postgres`)
- `routes` — HTTP/TCP routes (`${CLUSTER_DOMAIN}` expanded; `auto_tls` for ACME)
- `webhooks` — `flynn-host` webhooks (`url`, `secret_env`) registered on install
- `wait` — URL the installer polls before `hooks.ready`
- `hooks.install` / `hooks.upgrade` / `hooks.uninstall` / `hooks.ready` — optional scripts
- `build.entrypoint` — Flynn ImageManifest args
- `build.base` — Flynn GitHub repo/tag/`images.json` key for ubuntu-noble (`version: latest` or a pin like `v20260911.0`)
- `build.packages` — chroot apt overlay on that base
- `build.go` / `build.copy` — binaries and scripts installed into the delta layer
- `artifacts.image` — filled by `plugin-build` / the release workflow

Flynn APIs are declared in `go.mod` as `require github.com/randy-girard/flynn`.
Builds use `-mod=mod` (no `vendor/` directory). Do not pin a sibling `../flynn` replace.

## Install

```text
sudo flynn-host plugin:install example
sudo flynn-host plugin:install /path/to/this-repo
sudo flynn-host plugin:install https://github.com/OWNER/flynn-plugin-example.git --ref v20260914.0.0
sudo flynn-host plugin:update example --ref v20260914.0.0
sudo flynn-host plugin:uninstall example
```

The user `flynn` CLI never installs plugins. After install it shows commands
listed in that cluster's CLI catalog when `cli.doc` and `cli.actions` are set
(`cli` is optional; an `app` plugin may have none).
