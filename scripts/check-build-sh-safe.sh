#!/usr/bin/env sh
set -eu

repo_root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
build_script="$repo_root/build.sh"

forbidden_pattern='ffdfgdfg/|:latest|(^|[[:space:]])docker[[:space:]]|--push([[:space:]]|$)|push[[:space:]]*=[[:space:]]*true'

if LC_ALL=C grep -En "$forbidden_pattern" "$build_script"; then
  echo "build.sh must remain local-only and must not contain Docker commands or obsolete tags." >&2
  exit 1
fi

for workflow in release-packages.yml docker-publish-next.yml; do
  if ! grep -Fq ".github/workflows/$workflow" "$build_script"; then
    echo "build.sh must direct releases through .github/workflows/$workflow." >&2
    exit 1
  fi
done

echo "build.sh publishing guard passed."
