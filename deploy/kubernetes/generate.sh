#!/usr/bin/env bash
# Copyright (c) 2026 McSparrow. All rights reserved.
# McHarbor is licensed under the McHarbor License. See LICENSE for details.

set -euo pipefail

here="$(cd "$(dirname "$0")" && pwd)"
deploy="$(cd "$here/.." && pwd)"
helm_image="${HELM_IMAGE:-alpine/helm:3.16.4}"

run_helm() {
  if command -v helm >/dev/null 2>&1; then
    (cd "$deploy" && helm "$@")
  else
    local mount="$deploy"
    if command -v cygpath >/dev/null 2>&1; then
      mount="$(cygpath -m "$deploy")"
    fi
    MSYS_NO_PATHCONV=1 docker run --rm -v "$mount:/deploy" -w /deploy "$helm_image" "$@"
  fi
}

header='# Copyright (c) 2026 McSparrow. All rights reserved.
# McHarbor is licensed under the McHarbor License. See LICENSE for details.
'

render() {
  local values="$1" out="$2"
  run_helm lint helm/mcharbor -f "kubernetes/$values" >/dev/null
  {
    printf '%s\n' "$header"
    run_helm template mcharbor helm/mcharbor -f "kubernetes/$values" | tr -d '\r'
  } > "$here/$out"
  echo "wrote $here/$out"
}

render values-single-node.yaml single-node.yaml
render values-active-passive.yaml active-passive.yaml
