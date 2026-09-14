#!/usr/bin/env bash
# Runs on a cluster host during `flynn-host plugin install`, before the plugin
# app is deployed. Use this for cluster-specific setup (routes, secrets, waiting
# on postgres). The installer exports CONTROLLER_KEY, CLUSTER_DOMAIN,
# FLYNN_PLUGIN_NAME, FLYNN_PLUGIN_KIND, and FLYNN_PLUGIN_ROOT.
#
# Must be idempotent: plugin update re-runs this when hooks.upgrade is unset.
# A non-zero exit aborts install.
set -euo pipefail

: "${FLYNN_PLUGIN_NAME:?}"
: "${FLYNN_PLUGIN_KIND:?}"

# This template app needs no extra cluster setup.
exit 0
