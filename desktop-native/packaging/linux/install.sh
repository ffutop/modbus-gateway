#!/bin/sh
# Add ModMux to the desktop's application menu, with its icon, for the current
# user. The desktop entry runs modmux-desktop from this directory, so run this
# again after moving the directory; it replaces the previous entry.
#
#   ./install.sh               install the entry and icons
#   ./install.sh --uninstall   remove them
set -eu

app_id=com.ffutop.modmux.native
here=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
data_home=${XDG_DATA_HOME:-"$HOME/.local/share"}
entry="$data_home/applications/$app_id.desktop"
icons="$data_home/icons/hicolor"

refresh() {
    if command -v update-desktop-database >/dev/null 2>&1; then
        update-desktop-database -q "$data_home/applications" 2>/dev/null || :
    fi
    if command -v gtk-update-icon-cache >/dev/null 2>&1; then
        gtk-update-icon-cache -q -t "$icons" 2>/dev/null || :
    fi
}

case "${1:-}" in
"") ;;
--uninstall)
    rm -f "$entry"
    for icon in "$icons"/*/apps/"$app_id.png"; do
        rm -f "$icon"
    done
    refresh
    echo "Removed ModMux from the application menu."
    exit 0
    ;;
*)
    echo "usage: $0 [--uninstall]" >&2
    exit 2
    ;;
esac

for icon in "$here"/share/icons/hicolor/*/apps/"$app_id.png"; do
    size=$(basename "$(dirname "$(dirname "$icon")")")
    mkdir -p "$icons/$size/apps"
    cp "$icon" "$icons/$size/apps/$app_id.png"
done

# Exec takes a quoted path. Following the desktop entry spec: escape \ " ` $
# inside the quotes, then escape every backslash again for the string type,
# and double % so it is not a field code. Finally escape the sed replacement's
# own \ & and | delimiter.
exec_path=$(printf '%s' "$here/modmux-desktop" |
    sed -e 's/[\\"`$]/\\&/g' -e 's/\\/\\\\/g' -e 's/%/%%/g' -e 's/^/"/' -e 's/$/"/' \
        -e 's/[\\&|]/\\&/g')
mkdir -p "$(dirname "$entry")"
sed "s|@EXEC@|$exec_path|" "$here/share/applications/$app_id.desktop" > "$entry.tmp"
mv "$entry.tmp" "$entry"
refresh
echo "Added ModMux to the application menu: $entry"
