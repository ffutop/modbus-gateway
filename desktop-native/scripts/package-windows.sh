#!/bin/sh
# Build the Windows app; it needs no CGO, so any host can cross-compile it.
set -eu

. "$(dirname -- "$0")/common.sh"

stage=$(stage_dir windows)
name="modmux-desktop-windows-$arch"
mkdir "$stage/$name"
# windowsgui: a window app, so double-clicking opens no console.
CGO_ENABLED=0 GOOS=windows GOARCH="$arch" \
    go build -trimpath -ldflags "$ldflags -H=windowsgui" -o "$stage/$name/modmux-desktop.exe" .
copy_shared "$stage/$name"

archive="$output_dir/$name.zip"
rm -f "$archive"
(cd "$stage" && zip -q -r "$archive" "$name")
printf '\nBuilt: %s\nArchive: %s\n' "$stage/$name" "$archive"
