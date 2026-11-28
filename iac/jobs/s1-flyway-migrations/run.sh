#!/usr/bin/env bash
set -euo pipefail

namespace=${1:?namespace required}
image=${2:?image digest required}
run_key=${3:?run ID and attempt required}
if [[ ! $namespace =~ ^system1(-stage)?$ ||
      ! $image =~ ^ghcr\.io/umu-art/s1-flyway-migrations@sha256:[0-9a-f]{64}$ ||
      ! $run_key =~ ^[0-9]+-[0-9]+$ || ${#run_key} -gt 63 ]]; then
  echo 'Invalid migration Job arguments' >&2
  exit 2
fi

here=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
kube=(kubectl --namespace "$namespace")

sed "s/__NAMESPACE__/$namespace/g" "$here/external-secrets.yaml" |
  "${kube[@]}" apply -f -
for secret in s1-flyway-migrations github-container-registry; do
  "${kube[@]}" wait --for=condition=Ready "externalsecret/$secret" --timeout=120s
done

job=$(sed -e "s|__IMAGE__|$image|g" -e "s|__RUN_KEY__|$run_key|g" "$here/job.yaml" |
  "${kube[@]}" create -f - -o name)

fail_job() {
  echo "$job: $1" >&2
  "${kube[@]}" describe "$job" --request-timeout=15s >&2 || true
  "${kube[@]}" logs "$job" --request-timeout=15s >&2 || true
  if [[ ${2:-} == cancel ]]; then
    "${kube[@]}" delete "$job" --request-timeout=30s --wait=false --ignore-not-found >&2 || true
  fi
  exit 1
}
trap 'fail_job "migration interrupted" cancel' INT TERM

end=$((SECONDS + ${FLYWAY_JOB_WAIT_SECONDS:-330}))
while :; do
  if ! conditions=$("${kube[@]}" get "$job" --request-timeout=15s \
    -o 'jsonpath={range .status.conditions[*]}{.type}={.status}{"\n"}{end}'); then
    fail_job 'could not read Job status' cancel
  fi
  if grep -qxE '(Failed|FailureTarget)=True' <<< "$conditions"; then
    fail_job 'migration failed'
  fi
  if grep -qx 'Complete=True' <<< "$conditions"; then
    echo "$job: migration complete"
    exit 0
  fi
  (( SECONDS < end )) || fail_job 'timed out waiting for migration' cancel
  sleep 3
done
