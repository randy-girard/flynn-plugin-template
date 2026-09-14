#!/bin/bash
set -e

export DEBIAN_FRONTEND=noninteractive

apt-get update -o Acquire::Retries=5
apt-get install -y --no-install-recommends \
  ca-certificates \
  curl

# shellcheck source=img/apt-slim-finish.sh
source "$(dirname "$0")/apt-slim-finish.sh"
