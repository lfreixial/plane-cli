#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/.."
audit_dir=$(mktemp -d)
trap 'rm -rf "$audit_dir"' EXIT

if [[ $(go list -m -f '{{.Version}}' github.com/mattn/go-localereader) != v0.0.1 ]]; then
  echo 'Review the go-localereader license exception for the new version.' >&2
  exit 1
fi

# Install for the host before selecting cross-compilation targets.
GOBIN="$audit_dir" go install github.com/google/go-licenses/v2@v2.0.1
for target_os in linux darwin windows; do
  for target_arch in amd64 arm64; do
    target_dir="$audit_dir/$target_os-$target_arch"
    CGO_ENABLED=0 GOOS="$target_os" GOARCH="$target_arch" \
      "$audit_dir/go-licenses" save ./cmd/plane \
      --ignore github.com/lfreixial/plane-cli,github.com/mattn/go-localereader \
      --save_path "$target_dir"
    mkdir -p "$audit_dir/combined"
    cp -R "$target_dir/." "$audit_dir/combined/"
    chmod -R u+w "$audit_dir/combined"
  done
done

# go-localereader v0.0.1 declares MIT and its author in README.md, but ships
# no standalone license file. Preserve that declaration and the later upstream
# license file whose provenance is recorded in third_party/README.md.
reader_dir=$(go list -m -f '{{.Dir}}' github.com/mattn/go-localereader)
mkdir -p "$audit_dir/combined/github.com/mattn/go-localereader"
cp "$reader_dir/README.md" "$audit_dir/combined/github.com/mattn/go-localereader/README.md"
cp third_party/go-localereader-LICENSE "$audit_dir/combined/github.com/mattn/go-localereader/LICENSE"

# go-licenses excludes the standard library.
mkdir -p "$audit_dir/combined/go"
cp "$(go env GOROOT)/LICENSE" "$audit_dir/combined/go/LICENSE"
go version > "$audit_dir/combined/go/VERSION.txt"
go list -m all > "$audit_dir/combined/MODULES.txt"

# Replace generated output only after every target was collected successfully.
mkdir -p third_party/licenses
chmod -R u+w third_party/licenses
cp -R "$audit_dir/combined/." third_party/licenses/
echo 'License notices collected in third_party/licenses.'
