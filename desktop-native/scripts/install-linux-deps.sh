#!/bin/sh
# Install Gio's Linux build dependencies on Ubuntu (https://gioui.org/doc/install/linux).
# Each extra architecture given (amd64 or arm64) adds a cross compiler and that
# architecture's libraries through multiarch, for package-linux.sh to use.
#
#   sudo ./scripts/install-linux-deps.sh          # build for this host only
#   sudo ./scripts/install-linux-deps.sh arm64    # also cross-compile arm64
set -eu

# The C library and the libraries Gio's cgo directives name through pkg-config.
libs="libc6-dev libegl-dev libgles-dev libvulkan-dev libwayland-dev libx11-dev libx11-xcb-dev
libxcursor-dev libxfixes-dev libxkbcommon-dev libxkbcommon-x11-dev"

host=$(dpkg --print-architecture)
codename=$(. /etc/os-release && echo "$VERSION_CODENAME")
packages="gcc pkg-config $libs"

for arch in "$@"; do
    [ "$arch" = "$host" ] && continue
    case "$arch" in
    amd64)
        triplet=x86_64-linux-gnu
        mirror=http://archive.ubuntu.com/ubuntu
        security=http://security.ubuntu.com/ubuntu
        ;;
    arm64)
        triplet=aarch64-linux-gnu
        mirror=http://ports.ubuntu.com/ubuntu-ports
        security=$mirror
        ;;
    *)
        echo "unsupported architecture: $arch (want amd64 or arm64)" >&2
        exit 1
        ;;
    esac
    # Ubuntu publishes amd64 and arm64 from different archives (ports), so
    # pin the configured Ubuntu sources to the host architecture and add the
    # other archive for the extra one.
    if [ -f /etc/apt/sources.list ]; then
        sed -i -E \
            -e "s/^deb (https?|mirror\+file):/deb [arch=$host] \1:/" \
            -e "/^deb \[[^]]*arch=/!s/^deb \[/deb [arch=$host /" \
            /etc/apt/sources.list
    fi
    if [ -f /etc/apt/sources.list.d/ubuntu.sources ] &&
        ! grep -q '^Architectures:' /etc/apt/sources.list.d/ubuntu.sources; then
        sed -i "/^Types:/a Architectures: $host" /etc/apt/sources.list.d/ubuntu.sources
    fi
    cat > "/etc/apt/sources.list.d/modmux-$arch.list" <<LIST
deb [arch=$arch] $mirror $codename main universe
deb [arch=$arch] $mirror $codename-updates main universe
deb [arch=$arch] $security $codename-security main universe
LIST
    dpkg --add-architecture "$arch"
    packages="$packages gcc-$triplet $(printf '%s\n' $libs | sed "s/\$/:$arch/" | tr '\n' ' ')"
done

apt-get update
# shellcheck disable=SC2086 # word splitting intended
DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends $packages
