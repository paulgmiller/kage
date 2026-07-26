#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
image_name="kage-e2e:local"

docker build \
  --file "${repo_root}/test/e2e/Dockerfile" \
  --tag "${image_name}" \
  "${repo_root}"

docker run \
  --rm \
  --network host \
  --volume /var/run/docker.sock:/var/run/docker.sock \
  "${image_name}"
