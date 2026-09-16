#!/usr/bin/env bash
# Runs on a cluster host during `flynn-host plugin uninstall`, before the
# plugin app is deleted. Flynn already removes plugin webhooks, HTTP/TCP
# routes, and exclusive resources. This hook is idempotent extra cleanup.
# A non-zero exit aborts uninstall.
set -euo pipefail

: "${FLYNN_PLUGIN_NAME:?}"
: "${FLYNN_PLUGIN_KIND:?}"

echo "example plugin uninstall hook: app=${FLYNN_PLUGIN_APP:-example} kind=${FLYNN_PLUGIN_KIND}"
exit 0
