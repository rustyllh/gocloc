#!/usr/bin/env bash
# Build and test both release images locally. This script never logs in or pushes.
set -euo pipefail

if [[ $# -ne 2 ]]; then
    printf 'Usage: bash scripts/build-docker-release.sh RELEASE_DIR VERSION\n' >&2
    exit 1
fi
repo_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
release_dir=$(CDPATH= cd -- "$1" && pwd)
version=$2
if [[ ! "$version" =~ ^[0-9A-Za-z][0-9A-Za-z._+-]*$ ]]; then
    printf 'Invalid release version: %s\n' "$version" >&2
    exit 1
fi
revision=${GOCLOC_REVISION:-$(git -C "$repo_dir" rev-parse HEAD)}
context_dir=$(mktemp -d "${TMPDIR:-/tmp}/gocloc-docker.XXXXXX")
trap 'rm -rf -- "$context_dir"' EXIT
cp "$repo_dir/LICENSE" "$context_dir/LICENSE"

# Verify every downloaded archive before extracting the Linux executables.
(
    cd -- "$release_dir"
    if command -v sha256sum >/dev/null 2>&1; then
        sha256sum -c "gocloc_${version}_checksums.txt"
    else
        shasum -a 256 -c "gocloc_${version}_checksums.txt"
    fi
)

mkdir -p "$context_dir/input"
printf 'package sample\n' > "$context_dir/input/a.go"
cp "$context_dir/input/a.go" "$context_dir/input/b.go"

for arch in amd64 arm64; do
    asset_arch=$arch
    if [[ "$arch" == amd64 ]]; then asset_arch=x86_64; fi
    mkdir -p "$context_dir/$arch"
    tar -xOzf "$release_dir/gocloc_Linux_${asset_arch}.tar.gz" gocloc > "$context_dir/$arch/gocloc"
    image="gocloc-release-candidate:$arch"
    docker buildx build --load --platform "linux/$arch" --target release \
        --build-arg "VERSION=$version" --build-arg "REVISION=$revision" \
        --tag "$image" --file "$repo_dir/Dockerfile" "$context_dir"

    platform=$(docker image inspect --format '{{.Os}}/{{.Architecture}}' "$image")
    if [[ "$platform" != "linux/$arch" ]]; then
        printf 'Unexpected image platform: %s\n' "$platform" >&2
        exit 1
    fi
    actual_version=$(docker run --rm --read-only --platform "linux/$arch" "$image" --version)
    if [[ "$actual_version" != "$version" && "$actual_version" != "$version "* ]]; then
        printf 'Unexpected image version: %s (want %s)\n' "$actual_version" "$version" >&2
        exit 1
    fi
    docker run --rm --read-only --platform "linux/$arch" \
        --mount "type=bind,src=$context_dir/input,dst=/workdir,readonly" "$image" -o json . |
        jq -e '.total.files == 2 and .total.code == 2' >/dev/null
    docker run --rm --read-only --platform "linux/$arch" \
        --mount "type=bind,src=$context_dir/input,dst=/workdir,readonly" "$image" --dedup -o json . |
        jq -e '.total.files == 1 and .total.code == 1' >/dev/null
    printf 'Verified %s (%s)\n' "$image" "$actual_version"
done
