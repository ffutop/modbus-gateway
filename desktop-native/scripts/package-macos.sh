#!/bin/sh
# Build a native, locally signed .app for this Mac; no Go runtime is needed to run it.
set -eu

if [ "$(uname -s)" != "Darwin" ]; then
    echo "This script requires macOS and Xcode command line tools." >&2
    exit 1
fi

native_root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
output_dir=${1:-"$native_root/dist"}
mkdir -p "$output_dir"
output_dir=$(CDPATH= cd -- "$output_dir" && pwd)
bundle_path="$output_dir/ModMux.app"
mkdir -p "$bundle_path/Contents/MacOS" "$bundle_path/Contents/Resources"

cd "$native_root"
go build -trimpath -o "$bundle_path/Contents/MacOS/modmux-desktop" .
cp packaging/macos/Info.plist "$bundle_path/Contents/Info.plist"
cp packaging/macos/config.default.yaml "$bundle_path/Contents/Resources/config.default.yaml"
cp packaging/macos/ModMux-devtools-v3.icns "$bundle_path/Contents/Resources/ModMux.icns"
cp ../LICENSE "$bundle_path/Contents/Resources/LICENSE"
cp internal/uifont/assets/OFL.txt "$bundle_path/Contents/Resources/NotoSansSC-OFL.txt"
printf 'APPL????' > "$bundle_path/Contents/PkgInfo"
/usr/bin/plutil -lint "$bundle_path/Contents/Info.plist"
/usr/bin/codesign --force --sign - "$bundle_path"
/usr/bin/codesign --verify --strict --verbose=2 "$bundle_path"
printf '\nBuilt: %s\nDrag ModMux.app into Applications to install.\n' "$bundle_path"
