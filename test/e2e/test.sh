#!/usr/bin/env bash
set -euo pipefail

cluster_name="kage-e2e"
work_dir="$(mktemp -d)"

cleanup() {
  kind delete cluster --name "${cluster_name}" >/dev/null 2>&1 || true
  rm -rf "${work_dir}"
}
trap cleanup EXIT

export HOME="${work_dir}/home"
mkdir -p "${HOME}/.ssh" "${work_dir}/secrets"
kind delete cluster --name "${cluster_name}" >/dev/null 2>&1 || true

ssh-keygen \
  -q \
  -t ed25519 \
  -N "" \
  -f "${HOME}/.ssh/id_ed25519"
cp "${HOME}/.ssh/id_ed25519.pub" "${work_dir}/secrets/recipients.txt"

kage \
  --secret-file "${work_dir}/secrets/envtest" \
  --set "first-secret/API_TOKEN=first-secret-value"

kage \
  --secret-file "${work_dir}/secrets/envtest" \
  --set "second-secret/PASSWORD=second-secret-value"

check_output="$(
  kage \
    --secret-file "${work_dir}/secrets/envtest" \
    --check
)"
grep -q '^first-secret$' <<<"${check_output}"
grep -q '^  API_TOKEN=f\\[[0-9][0-9]*\\]e$' <<<"${check_output}"
grep -q '^second-secret$' <<<"${check_output}"
grep -q '^  PASSWORD=s\\[[0-9][0-9]*\\]e$' <<<"${check_output}"

kind create cluster --name "${cluster_name}" --wait 120s
kage \
  --secret-file "${work_dir}/secrets/envtest" \
  --ns default \
  --apply

test "$(
  kubectl get secret first-secret \
    --namespace default \
    --output 'jsonpath={.data.API_TOKEN}' \
    | base64 --decode
)" = "first-secret-value"
test "$(
  kubectl get secret second-secret \
    --namespace default \
    --output 'jsonpath={.data.PASSWORD}' \
    | base64 --decode
)" = "second-secret-value"

echo "kage end-to-end test passed"
