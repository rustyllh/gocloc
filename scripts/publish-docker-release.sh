#!/usr/bin/env bash
# Upload tested images by digest, then expose only multi-platform release tags.
set -euo pipefail

if [[ $# -ne 2 ]]; then
    printf 'Usage: bash scripts/publish-docker-release.sh IMAGE RELEASE_TAG\n' >&2
    exit 1
fi
image=$1
release_tag=$2
if [[ ! "$release_tag" =~ ^v[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z][0-9A-Za-z.-]*)?$ ]]; then
    printf 'Invalid Docker release tag: %s\n' "$release_tag" >&2
    exit 1
fi

archive_dir=$(mktemp -d "${TMPDIR:-/tmp}/gocloc-docker-publish.XXXXXX")
trap 'rm -rf -- "$archive_dir"' EXIT
references=()
for arch in amd64 arm64; do
    archive="$archive_dir/$arch.tar"
    docker image save --output "$archive" "gocloc-release-candidate:$arch"
    digest=$(crane digest --tarball "$archive")
    if [[ ! "$digest" =~ ^sha256:[0-9a-f]{64}$ ]]; then
        printf 'Invalid image digest: %s\n' "$digest" >&2
        exit 1
    fi
    reference="$image@$digest"
    crane push "$archive" "$reference"
    references+=("$reference")
done

tags=(--tag "$image:$release_tag")
if [[ "$release_tag" != *-* ]]; then
    tags+=(--tag "$image:latest")
fi
docker buildx imagetools create "${tags[@]}" "${references[@]}"
docker buildx imagetools inspect "$image:$release_tag" --raw |
    jq -e '([.manifests[].platform | .os + "/" + .architecture] | sort) == ["linux/amd64", "linux/arm64"]'
