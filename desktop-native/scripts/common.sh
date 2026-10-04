# Shared by the package-*.sh scripts; sourced, not run.
#
# Environment:
#   VERSION  release version embedded in the binary (default: dev), e.g. v0.6.1
#   GOARCH   target architecture, amd64 or arm64 (default: the Go toolchain's)
# Argument:
#   $1       output directory (default: dist)
#
# Each script leaves modmux-desktop-<goos>-<goarch>.<zip|tar.gz> in the output
# directory and its unpacked contents under <goos>-<goarch>/.

native_root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
version=${VERSION:-dev}
arch=${GOARCH:-$(go env GOARCH)}
case "$arch" in
amd64 | arm64) ;;
*)
    echo "unsupported GOARCH: $arch (want amd64 or arm64)" >&2
    exit 1
    ;;
esac
output_dir=${1:-"$native_root/dist"}
mkdir -p "$output_dir"
output_dir=$(CDPATH= cd -- "$output_dir" && pwd)
ldflags="-s -w -X main.version=$version"

# stage_dir GOOS: an empty directory for this target's unpacked contents.
stage_dir() {
    stage="$output_dir/$1-$arch"
    rm -rf "$stage"
    mkdir -p "$stage"
    printf '%s' "$stage"
}

# copy_shared DIR: the first-run sample and licenses every package ships.
copy_shared() {
    cp "$native_root/packaging/config.default.yaml" "$1/config.default.yaml"
    cp "$native_root/../LICENSE" "$1/LICENSE"
    cp "$native_root/internal/uifont/assets/OFL.txt" "$1/NotoSansSC-OFL.txt"
}

cd "$native_root"
