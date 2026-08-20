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
  set \
  --secret-file "${work_dir}/secrets/envtest" \
  "first-secret/API_TOKEN=first-secret-value"

kage \
  set \
  --secret-file "${work_dir}/secrets/envtest" \
  "second-secret/PASSWORD=second-secret-value"

check_output="$(
  kage \
    check \
    --secret-file "${work_dir}/secrets/envtest"
)"
expected_check_output=$'first-secret\n  API_TOKEN=f[18]e\n\nsecond-secret\n  PASSWORD=s[19]e'
if [[ "${check_output}" != "${expected_check_output}" ]]; then
  echo "unexpected kage --check output" >&2
  printf 'expected:\n%s\nactual:\n%s\n' \
    "${expected_check_output}" \
    "${check_output}" >&2
  exit 1
fi

kind create cluster --name "${cluster_name}" --wait 120s
kage \
  apply \
  --secret-file "${work_dir}/secrets/envtest" \
  --namespace default \
  --confirm

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

exported_file="${work_dir}/secrets/namespace-export"
kage \
  create \
  --secret-file "${exported_file}" \
  --namespace default

exported_check_output="$(
  kage \
    check \
    --secret-file "${exported_file}"
)"
if [[ "${exported_check_output}" != "${expected_check_output}" ]]; then
  echo "unexpected check output for namespace export" >&2
  printf 'expected:\n%s\nactual:\n%s\n' \
    "${expected_check_output}" \
    "${exported_check_output}" >&2
  exit 1
fi

echo "kage end-to-end test passed"
