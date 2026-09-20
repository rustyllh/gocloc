#!/usr/bin/env bash
# Exercise release orchestration and failure gates without Docker or network access.
set -euo pipefail
repo_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
test_dir=$(mktemp -d "${TMPDIR:-/tmp}/gocloc-docker-test.XXXXXX")
trap 'rm -rf -- "$test_dir"' EXIT
mkdir -p "$test_dir/mockbin" "$test_dir/releases" "$test_dir/input"
cp "$repo_dir/scripts/testdata/docker-release/docker" "$test_dir/mockbin/docker"
cp "$repo_dir/scripts/testdata/docker-release/crane" "$test_dir/mockbin/crane"
chmod +x "$test_dir/mockbin/docker" "$test_dir/mockbin/crane"
export PATH="$test_dir/mockbin:$PATH"
export GOCLOC_DOCKER_TEST_LOG="$test_dir/docker.log"
export GOCLOC_DOCKER_TEST_FAILURE=
export GOCLOC_REVISION=fixture

for arch in amd64 arm64; do
    asset_arch=$arch
    if [[ "$arch" == amd64 ]]; then asset_arch=x86_64; fi
    asset="gocloc_Linux_${asset_arch}.tar.gz"
    printf 'fixture-%s\n' "$arch" > "$test_dir/input/gocloc"
    tar -czf "$test_dir/releases/$asset" -C "$test_dir/input" gocloc
    (
        cd -- "$test_dir/releases"
        if command -v sha256sum >/dev/null 2>&1; then
            sha256sum "$asset"
        else
            shasum -a 256 "$asset"
        fi
    ) >> "$test_dir/releases/gocloc_1.2.3_checksums.txt"
done

run_case() {
    local name=$1 expected=$2 status=0
    : > "$GOCLOC_DOCKER_TEST_LOG"
    bash "$repo_dir/scripts/build-docker-release.sh" "$test_dir/releases" 1.2.3 \
        > "$test_dir/output" 2>&1 || status=$?
    if [[ "$status" -ne "$expected" ]]; then
        printf 'FAIL: %s (exit %s, expected %s)\n' "$name" "$status" "$expected" >&2
        cat "$test_dir/output" >&2
        exit 1
    fi
    printf 'PASS: %s\n' "$name"
}

run_case success 0
[[ $(grep -c '^buildx build ' "$GOCLOC_DOCKER_TEST_LOG") -eq 2 ]]
[[ $(grep -c '^run --rm ' "$GOCLOC_DOCKER_TEST_LOG") -eq 6 ]]
grep -F 'gocloc-release-candidate:amd64' "$GOCLOC_DOCKER_TEST_LOG" >/dev/null
grep -F 'gocloc-release-candidate:arm64' "$GOCLOC_DOCKER_TEST_LOG" >/dev/null

for failure in platform version statistics; do
    GOCLOC_DOCKER_TEST_FAILURE=$failure
    run_case "$failure" 1
    # Failure on amd64 must stop before attempting the second image.
    [[ $(grep -c '^buildx build ' "$GOCLOC_DOCKER_TEST_LOG") -eq 1 ]]
done
GOCLOC_DOCKER_TEST_FAILURE=
printf 'tampered archive\n' >> "$test_dir/releases/gocloc_Linux_arm64.tar.gz"
run_case checksum 1
[[ ! -s "$GOCLOC_DOCKER_TEST_LOG" ]]

run_publish_case() {
    local name=$1 tag=$2 expected=$3 status=0
    : > "$GOCLOC_DOCKER_TEST_LOG"
    bash "$repo_dir/scripts/publish-docker-release.sh" rustyllh/gocloc "$tag" \
        > "$test_dir/output" 2>&1 || status=$?
    if [[ "$status" -ne "$expected" ]]; then
        printf 'FAIL: %s (exit %s, expected %s)\n' "$name" "$status" "$expected" >&2
        cat "$test_dir/output" >&2
        exit 1
    fi
    printf 'PASS: %s\n' "$name"
}

amd64_ref=$(printf 'rustyllh/gocloc@sha256:%064d' 1)
arm64_ref=$(printf 'rustyllh/gocloc@sha256:%064d' 2)
run_publish_case stable-tags v1.2.3 0
[[ $(grep -c '^crane push ' "$GOCLOC_DOCKER_TEST_LOG") -eq 2 ]]
grep -Fx "buildx imagetools create --tag rustyllh/gocloc:v1.2.3 --tag rustyllh/gocloc:latest $amd64_ref $arm64_ref" \
    "$GOCLOC_DOCKER_TEST_LOG" >/dev/null

run_publish_case prerelease-tag v1.2.3-rc.1 0
grep -Fx "buildx imagetools create --tag rustyllh/gocloc:v1.2.3-rc.1 $amd64_ref $arm64_ref" \
    "$GOCLOC_DOCKER_TEST_LOG" >/dev/null
if grep -F 'latest' "$GOCLOC_DOCKER_TEST_LOG" >/dev/null; then exit 1; fi

run_publish_case invalid-tag latest 1
[[ ! -s "$GOCLOC_DOCKER_TEST_LOG" ]]
for failure in digest push; do
    GOCLOC_DOCKER_TEST_FAILURE=$failure
    run_publish_case "$failure" v1.2.3 1
    if grep -F 'buildx imagetools create' "$GOCLOC_DOCKER_TEST_LOG" >/dev/null; then exit 1; fi
done
GOCLOC_DOCKER_TEST_FAILURE=manifest
run_publish_case manifest v1.2.3 1
if grep -F 'buildx imagetools inspect' "$GOCLOC_DOCKER_TEST_LOG" >/dev/null; then exit 1; fi
printf 'All offline Docker release tests passed.\n'
