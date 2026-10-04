#!/bin/sh
# Build a native, locally signed .app; no Go runtime is needed to run it.
# A Mac builds both architectures: clang cross-compiles the Cocoa/Metal code.
set -eu

if [ "$(uname -s)" != "Darwin" ]; then
    echo "This script requires macOS and Xcode command line tools." >&2
    exit 1
fi

. "$(dirname -- "$0")/common.sh"

case "$arch" in
amd64) clang_arch=x86_64 ;;
arm64) clang_arch=arm64 ;;
esac
stage=$(stage_dir darwin)
bundle_path="$stage/ModMux.app"
mkdir -p "$bundle_path/Contents/MacOS" "$bundle_path/Contents/Resources"

CGO_ENABLED=1 GOOS=darwin GOARCH="$arch" CC="clang -arch $clang_arch" \
    go build -trimpath -ldflags "$ldflags" -o "$bundle_path/Contents/MacOS/modmux-desktop" .
cp packaging/macos/Info.plist "$bundle_path/Contents/Info.plist"
# Bundle versions must be numeric; a development build keeps the template's.
bundle_version=${version#v}
if printf '%s' "$bundle_version" | grep -Eq '^[0-9]+\.[0-9]+\.[0-9]+$'; then
    /usr/bin/plutil -replace CFBundleShortVersionString -string "$bundle_version" "$bundle_path/Contents/Info.plist"
    /usr/bin/plutil -replace CFBundleVersion -string "$bundle_version" "$bundle_path/Contents/Info.plist"
fi
copy_shared "$bundle_path/Contents/Resources"
cp packaging/macos/ModMux-devtools-v3.icns "$bundle_path/Contents/Resources/ModMux.icns"
printf 'APPL????' > "$bundle_path/Contents/PkgInfo"
/usr/bin/plutil -lint "$bundle_path/Contents/Info.plist"
/usr/bin/codesign --force --sign - "$bundle_path"
/usr/bin/codesign --verify --strict --verbose=2 "$bundle_path"

archive="$output_dir/modmux-desktop-darwin-$arch.zip"
rm -f "$archive"
# ditto keeps the signature and extended attributes that zip would drop.
/usr/bin/ditto -c -k --keepParent "$bundle_path" "$archive"
printf '\nBuilt: %s\nArchive: %s\nDrag ModMux.app into Applications to install.\n' "$bundle_path" "$archive"
