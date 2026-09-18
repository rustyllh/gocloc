#!/bin/sh
# All network calls and binaries are fixtures; never changes the user's install.
set -eu
repo_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
test_dir=$(mktemp -d)
trap 'rm -rf "$test_dir"' EXIT
trap 'exit 1' HUP INT TERM
mkdir -p "$test_dir/mockbin" "$test_dir/releases" "$test_dir/bin with spaces"
cp "$repo_dir/scripts/testdata/installer/curl" "$repo_dir/scripts/testdata/installer/uname" "$test_dir/mockbin/"
chmod +x "$test_dir/mockbin/curl" "$test_dir/mockbin/uname"
export GOCLOC_INSTALL_TEST_FIXTURES="$test_dir/releases"
export GOCLOC_INSTALL_TEST_OS=Linux
export GOCLOC_INSTALL_TEST_ARCH=x86_64
export GOCLOC_INSTALL_TEST_FAILURE=
export PATH="$test_dir/mockbin:$PATH"

for platform in Linux_x86_64 Linux_arm64 Linux_i386 Darwin_x86_64 Darwin_arm64; do
    asset=gocloc_${platform}.tar.gz
    tar -czf "$test_dir/releases/$asset" -C "$repo_dir/scripts/testdata/installer" gocloc
    if command -v sha256sum >/dev/null 2>&1; then
        checksum=$(sha256sum "$test_dir/releases/$asset")
    else
        checksum=$(shasum -a 256 "$test_dir/releases/$asset")
    fi
    printf '%s  %s\n' "${checksum%% *}" "$asset" >> "$test_dir/releases/gocloc_1.2.3_checksums.txt"
done

run_case() {
    name=$1
    expected_status=$2
    expected_message=$3
    shift 3
    status=0
    "${GOCLOC_INSTALL_TEST_SHELL:-sh}" "$repo_dir/install.sh" "$@" > "$test_dir/output" 2>&1 || status=$?
    if [ "$status" -ne "$expected_status" ] || ! grep -F "$expected_message" "$test_dir/output" >/dev/null; then
        printf 'FAIL: %s (exit %s, expected %s)\n' "$name" "$status" "$expected_status" >&2
        cat "$test_dir/output" >&2
        exit 1
    fi
    printf 'PASS: %s\n' "$name"
}

run_case help 0 'Usage:' --help
run_case invalid-option 1 'Unknown argument' --unknown
run_case missing-version 1 'requires a value' --version
run_case missing-directory 1 'requires a value' --install-dir
run_case invalid-version 1 'Invalid release version' --version '../bad'
run_case multiline-version 1 'Invalid release version' --version "$(printf 'v1.2.3\nbad')"
run_case latest 0 '1.2.3 (installer-test)' --install-dir "$test_dir/bin with spaces"
grep -F '/releases/latest' "$test_dir/releases/requests" >/dev/null
[ -x "$test_dir/bin with spaces/gocloc" ]
grep -F 'beginning of PATH' "$test_dir/output" >/dev/null

for platform in Linux:x86_64:Linux_x86_64 Linux:aarch64:Linux_arm64 Linux:i686:Linux_i386 Darwin:x86_64:Darwin_x86_64 Darwin:arm64:Darwin_arm64; do
    GOCLOC_INSTALL_TEST_OS=${platform%%:*}
    rest=${platform#*:}
    GOCLOC_INSTALL_TEST_ARCH=${rest%%:*}
    asset=gocloc_${rest#*:}.tar.gz
    printf '' > "$test_dir/releases/requests"
    run_case "$platform" 0 'Installed' --version 1.2.3 --install-dir "$test_dir/bin with spaces"
    grep -F "/v1.2.3/$asset" "$test_dir/releases/requests" >/dev/null
    if grep -F '/releases/latest' "$test_dir/releases/requests" >/dev/null; then exit 1; fi
done

GOCLOC_INSTALL_TEST_OS=Linux
GOCLOC_INSTALL_TEST_ARCH=riscv64
run_case unsupported-architecture 1 'Unsupported architecture' --version 1.2.3 --install-dir "$test_dir/bin with spaces"
GOCLOC_INSTALL_TEST_OS=FreeBSD
GOCLOC_INSTALL_TEST_ARCH=x86_64
run_case unsupported-os 1 'Unsupported OS' --version 1.2.3 --install-dir "$test_dir/bin with spaces"
GOCLOC_INSTALL_TEST_OS=Darwin
GOCLOC_INSTALL_TEST_ARCH=i386
run_case unsupported-target 1 'No release binary' --version 1.2.3 --install-dir "$test_dir/bin with spaces"
GOCLOC_INSTALL_TEST_OS=Linux
GOCLOC_INSTALL_TEST_ARCH=x86_64

# Every failed upgrade must preserve the previous executable.
printf 'existing installation\n' > "$test_dir/bin with spaces/gocloc"
for scenario in latest download corrupt missing duplicate binary; do
    GOCLOC_INSTALL_TEST_FAILURE=$scenario
    case "$scenario" in
        latest) message='Cannot find the latest release' ;;
        download) message='Cannot download' ;;
        corrupt) message='SHA-256 mismatch' ;;
        missing|duplicate) message='Missing or ambiguous' ;;
        binary) message='Downloaded binary cannot run' ;;
    esac
    run_case "$scenario" 1 "$message" --install-dir "$test_dir/bin with spaces"
    [ "$(cat "$test_dir/bin with spaces/gocloc")" = 'existing installation' ]
    [ -z "$(find "$test_dir/bin with spaces" -name '.gocloc.*' -print)" ]
done
GOCLOC_INSTALL_TEST_FAILURE=
mkdir -p "$test_dir/directory-target/gocloc"
run_case directory-target 1 'Refusing to replace' --version v1.2.3 --install-dir "$test_dir/directory-target"
mkdir -p "$test_dir/symlink-target"
ln -s "$test_dir/bin with spaces/gocloc" "$test_dir/symlink-target/gocloc"
run_case symlink-target 1 'Refusing to replace' --version v1.2.3 --install-dir "$test_dir/symlink-target"
[ "$(cat "$test_dir/bin with spaces/gocloc")" = 'existing installation' ]
run_case successful-upgrade 0 '1.2.3 (installer-test)' --version v1.2.3 --install-dir "$test_dir/bin with spaces"
printf 'All Unix installer tests passed.\n'
