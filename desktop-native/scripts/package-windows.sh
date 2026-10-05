#!/bin/sh
# Build the Windows app; it needs no CGO, so any host can cross-compile it.
set -eu

. "$(dirname -- "$0")/common.sh"

stage=$(stage_dir windows)
name="modmux-desktop-windows-$arch"
mkdir "$stage/$name"

# The icon and version are Windows resources linked from a .syso next to
# main.go; Gio gives the window the icon at resource ID 1. The .syso exists
# only for this build, so other targets never pick it up.
syso="$native_root/rsrc_windows_$arch.syso"
trap 'rm -f "$syso"' EXIT
set -- --icon packaging/icon/ModMux-devtools-v3.png --manifest none \
    --product-name ModMux --file-description ModMux \
    --original-filename modmux-desktop.exe
# Version resources must be numeric; a development build carries none.
file_version=${version#v}
if printf '%s' "$file_version" | grep -Eq '^[0-9]+\.[0-9]+\.[0-9]+$'; then
    set -- "$@" --product-version "$file_version" --file-version "$file_version"
fi
# The tool runs here, so build it for the host rather than the target.
GOOS=$(go env GOHOSTOS) GOARCH=$(go env GOHOSTARCH) \
    go run github.com/tc-hib/go-winres@v0.3.3 simply --arch "$arch" --out rsrc "$@"

# windowsgui: a window app, so double-clicking opens no console.
CGO_ENABLED=0 GOOS=windows GOARCH="$arch" \
    go build -trimpath -ldflags "$ldflags -H=windowsgui" -o "$stage/$name/modmux-desktop.exe" .
rm -f "$syso"
copy_shared "$stage/$name"

archive="$output_dir/$name.zip"
rm -f "$archive"
(cd "$stage" && zip -q -r "$archive" "$name")
printf '\nBuilt: %s\nArchive: %s\n' "$stage/$name" "$archive"
