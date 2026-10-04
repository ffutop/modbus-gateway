#!/bin/sh
# Build the Linux app. Gio needs CGO and the X11/Wayland/EGL/Vulkan development
# packages: install-linux-deps.sh installs them, and with an architecture
# argument the multiarch libraries and cross compiler that a GOARCH other than
# the host's uses here. Set CC and PKG_CONFIG_LIBDIR to use another toolchain.
set -eu

if [ "$(uname -s)" != "Linux" ]; then
    echo "This script requires Linux." >&2
    exit 1
fi

. "$(dirname -- "$0")/common.sh"

case "$(uname -m)" in
x86_64 | amd64) host=amd64 ;;
aarch64 | arm64) host=arm64 ;;
*) host=$(uname -m) ;;
esac
if [ "$arch" != "$host" ]; then
    case "$arch" in
    amd64) triplet=x86_64-linux-gnu ;;
    arm64) triplet=aarch64-linux-gnu ;;
    esac
    export CC="${CC:-$triplet-gcc}"
    # Only the target's .pc files, so pkg-config never offers host libraries.
    export PKG_CONFIG_LIBDIR="${PKG_CONFIG_LIBDIR:-/usr/lib/$triplet/pkgconfig:/usr/share/pkgconfig}"
fi

stage=$(stage_dir linux)
name="modmux-desktop-linux-$arch"
mkdir "$stage/$name"
CGO_ENABLED=1 GOOS=linux GOARCH="$arch" \
    go build -trimpath -ldflags "$ldflags" -o "$stage/$name/modmux-desktop" .
copy_shared "$stage/$name"

archive="$output_dir/$name.tar.gz"
tar -C "$stage" -czf "$archive" "$name"
printf '\nBuilt: %s\nArchive: %s\n' "$stage/$name" "$archive"
