#!/usr/bin/env bash
set -euo pipefail

export PATH="/usr/local/go/bin:${PATH}"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/../.." && pwd)"
cd "${REPO_ROOT}"

RUN_FRONTEND_DOCS=1
RUN_BACKEND=1

usage() {
  cat <<'EOF'
Usage: ./tests/e2e/run_e2e.sh [--backend-only | --frontend-docs-only]

Options:
  --backend-only        Run only the Go E2E suite (go test -v -race -tags=e2e ./tests/e2e/...)
  --frontend-docs-only  Run only the Node.js Docs/Frontend/Quality E2E suite
  -h, --help            Show this help message
EOF
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --backend-only)
      RUN_FRONTEND_DOCS=0
      RUN_BACKEND=1
      shift
      ;;
    --frontend-docs-only)
      RUN_FRONTEND_DOCS=1
      RUN_BACKEND=0
      shift
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "Unknown option: $1" >&2
      usage >&2
      exit 2
      ;;
  esac
done

node_rc=0
go_rc=0

if [[ "${RUN_FRONTEND_DOCS}" -eq 1 ]]; then
  echo "=== [E2E] Running Node.js Docs / Frontend / Quality Suite ==="
  set +e
  node --test tests/e2e/docs_frontend_quality.test.mjs
  node_rc=$?
  set -e
fi

if [[ "${RUN_BACKEND}" -eq 1 ]]; then
  echo "=== [E2E] Running Go Backend E2E Suite (-tags=e2e) ==="
  set +e
  go test -v -race -tags=e2e ./tests/e2e/...
  go_rc=$?
  set -e
fi

echo ""
echo "=== [E2E] Summary ==="
if [[ "${RUN_FRONTEND_DOCS}" -eq 1 ]]; then
  echo "  Node.js Docs/Frontend/Quality suite exit code: ${node_rc}"
fi
if [[ "${RUN_BACKEND}" -eq 1 ]]; then
  echo "  Go Backend E2E suite exit code:            ${go_rc}"
fi

if [[ "${node_rc}" -ne 0 || "${go_rc}" -ne 0 ]]; then
  exit 1
fi
exit 0
