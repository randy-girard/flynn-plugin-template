#!/usr/bin/env bash
# Embed web/dist for plugin-build. Skip npm ci / npm run build when sources
# have not changed and dist/index.html already exists. Re-run npm ci when the
# lockfile, OS/arch, or rollup native binding does not match this machine
# (host node_modules copied into Linux plugin-build is the usual miss).
#
# shellcheck shell=bash

plugin_web_fingerprint() {
  local web="$1"
  python3 - "$web" <<'PY'
import hashlib, os, sys

root = sys.argv[1]
skip_dirs = {"node_modules", "dist", "coverage"}
files = []
for dirpath, dirs, names in os.walk(root):
    dirs[:] = [d for d in dirs if d not in skip_dirs]
    for name in names:
        if name == ".DS_Store":
            continue
        path = os.path.join(dirpath, name)
        rel = os.path.relpath(path, root)
        files.append(rel)
h = hashlib.sha256()
for rel in sorted(files):
    path = os.path.join(root, rel)
    h.update(rel.encode())
    h.update(b"\0")
    with open(path, "rb") as f:
        for chunk in iter(lambda: f.read(65536), b""):
            h.update(chunk)
    h.update(b"\n")
print(h.hexdigest())
PY
}

plugin_file_hash() {
  python3 - "$1" <<'PY'
import hashlib, sys
h = hashlib.sha256()
with open(sys.argv[1], "rb") as f:
    for chunk in iter(lambda: f.read(65536), b""):
        h.update(chunk)
print(h.hexdigest())
PY
}

# OS/arch token used in the npm skip stamp and @rollup/rollup-* package name.
plugin_npm_platform() {
  if command -v node >/dev/null 2>&1; then
    node -e '
      const p = process.platform, a = process.arch;
      let extra = "";
      if (p === "linux") {
        let musl = false;
        try { musl = !process.report.getReport().header.glibcVersionRuntime; } catch (e) {}
        extra = musl ? "-musl" : "-gnu";
        if (a === "arm") extra = musl ? "-musleabihf" : "-gnueabihf";
      } else if (p === "win32") {
        extra = "-msvc";
      }
      process.stdout.write(p + "-" + a + extra);
    '
    return
  fi
  echo "$(uname -s)-$(uname -m)"
}

plugin_rollup_native_ok() {
  local web="$1"
  [[ -d "${web}/node_modules/rollup" ]] || return 0
  command -v node >/dev/null 2>&1 || return 1
  (cd "${web}" && node -e "require('rollup/dist/native.js')" >/dev/null 2>&1)
}

plugin_ensure_rollup_native() {
  local web="$1"
  [[ -d "${web}/node_modules/rollup" ]] || return 0
  if plugin_rollup_native_ok "${web}"; then
    return 0
  fi
  local pkg ver
  pkg="@rollup/rollup-$(plugin_npm_platform)"
  ver="$(cd "${web}" && node -p "require('./node_modules/rollup/package.json').version")"
  echo "==> npm install ${pkg}@${ver} (rollup native binding)"
  (cd "${web}" && npm install --no-save --no-package-lock --include=optional "${pkg}@${ver}")
  if ! plugin_rollup_native_ok "${web}"; then
    echo "rollup native binding ${pkg} is still missing after npm install" >&2
    return 1
  fi
}

plugin_embed_web() {
  local root="${1:-.}"
  local web="${root}/web"
  if [[ ! -f "${web}/package.json" ]]; then
    return 0
  fi
  if ! command -v npm >/dev/null 2>&1; then
    if command -v apt-get >/dev/null 2>&1; then
      echo "npm missing; installing nodejs from apt"
      export DEBIAN_FRONTEND=noninteractive
      if [[ "$(id -u)" -eq 0 ]]; then
        apt-get update -o Acquire::Retries=5
        apt-get install -y --no-install-recommends nodejs npm
      else
        sudo apt-get update -o Acquire::Retries=5
        sudo apt-get install -y --no-install-recommends nodejs npm
      fi
    fi
  fi
  if ! command -v npm >/dev/null 2>&1; then
    echo "npm is required to embed the plugin UI" >&2
    return 1
  fi

  local cache="${FLYNN_PLUGIN_BUILD_CACHE:-${root}/.plugin-build-cache}"
  mkdir -p "${cache}"
  local stamp="${cache}/web.fingerprint"
  local lock_stamp="${cache}/web.lockhash"
  local fingerprint
  fingerprint="$(plugin_web_fingerprint "${web}")"
  local force="${FLYNN_PLUGIN_BUILD_FORCE_WEB:-}"

  if [[ -z "${force}" && -f "${web}/dist/index.html" && -f "${stamp}" && "$(cat "${stamp}")" == "${fingerprint}" ]]; then
    echo "==> web/ unchanged; reusing dist/"
    return 0
  fi

  local lockhash=""
  if [[ -f "${web}/package-lock.json" ]]; then
    lockhash="$(plugin_file_hash "${web}/package-lock.json")"
  elif [[ -f "${web}/package.json" ]]; then
    lockhash="$(plugin_file_hash "${web}/package.json")"
  fi
  local platform want
  platform="$(plugin_npm_platform)"
  want="${lockhash} ${platform}"
  if [[ ! -d "${web}/node_modules" || -z "${lockhash}" || ! -f "${lock_stamp}" || "$(cat "${lock_stamp}")" != "${want}" ]] || ! plugin_rollup_native_ok "${web}"; then
    echo "==> npm ci --include=optional (web/ ${platform})"
    (cd "${web}" && npm ci --include=optional)
    plugin_ensure_rollup_native "${web}"
    printf '%s\n' "${want}" > "${lock_stamp}"
  else
    echo "==> npm ci skipped (lockfile unchanged)"
  fi

  echo "==> npm run build (web/)"
  (cd "${web}" && npm run build)
  printf '%s\n' "${fingerprint}" > "${stamp}"
}
