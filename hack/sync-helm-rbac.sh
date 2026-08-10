#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SOURCE="${ROOT_DIR}/config/rbac/role.yaml"
DESTINATION="${ROOT_DIR}/charts/external-service-discovery-operator/generated/manager-role-rules.yaml"

if [[ ! -f "${SOURCE}" ]]; then
  echo "Generated manager RBAC does not exist: ${SOURCE}" >&2
  exit 1
fi

mkdir -p "$(dirname "${DESTINATION}")"

awk '
  found { print }
  /^rules:$/ { found = 1 }
  END {
    if (!found) {
      print "Generated manager RBAC has no rules section." > "/dev/stderr"
      exit 1
    }
  }
' "${SOURCE}" > "${DESTINATION}"

if [[ ! -s "${DESTINATION}" ]]; then
  echo "Generated Helm RBAC rules are empty: ${DESTINATION}" >&2
  exit 1
fi

echo "Synchronized generated manager RBAC into the Helm chart"
