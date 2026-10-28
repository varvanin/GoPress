#!/bin/sh
set -eu
release_version=${1:?Usage: scripts/package-release.sh VERSION}
case "$release_version" in *[!a-zA-Z0-9._-]*|'') echo 'Invalid version' >&2; exit 1;; esac
mkdir -p dist
for release_arch in amd64 arm64; do
  release_name="gopress-${release_version}-linux-${release_arch}"
  CGO_ENABLED=0 GOOS=linux GOARCH="$release_arch" go build -trimpath -ldflags="-s -w -X gopress/internal/version.Current=$release_version" -o "dist/$release_name" ./cmd/gopress
  tar -czf "dist/$release_name.tar.gz" -C dist "$release_name" -C .. README.md LICENSE AGENTS.md WORK_PLAN.md GOPRESS_PLAN.md docs internal/httpui/starter/README.md internal/httpui/starter/TEMPLATE_REFERENCE.md internal/httpui/module-starter/README.md
  rm "dist/$release_name"
done
if command -v sha256sum >/dev/null 2>&1; then
  (cd dist && sha256sum "gopress-${release_version}-linux-"*.tar.gz > "SHA256SUMS-${release_version}")
else
  (cd dist && shasum -a 256 "gopress-${release_version}-linux-"*.tar.gz > "SHA256SUMS-${release_version}")
fi
