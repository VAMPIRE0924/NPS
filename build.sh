#!/usr/bin/env bash
set -euo pipefail

repo_root=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
cd "$repo_root"

target_os=$(go env GOOS)
target_arch=$(go env GOARCH)
output_dir="$repo_root/dist/local-${target_os}-${target_arch}"
binary_name=nps
if [[ "$target_os" == windows ]]; then
  binary_name=nps.exe
fi

mkdir -p "$output_dir/web"
CGO_ENABLED=0 go build -buildvcs=false -trimpath -ldflags='-s -w' \
  -o "$output_dir/$binary_name" ./cmd/nps
cp -R web/static web/views "$output_dir/web/"

printf '%s\n' \
  "Local NPS build is available at $output_dir" \
  'Publishing is disabled in build.sh.' \
  'Use .github/workflows/release-packages.yml for GitHub Release assets.' \
  'Use .github/workflows/docker-publish-next.yml for Docker images.'
