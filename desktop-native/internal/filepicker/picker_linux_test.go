package filepicker

import (
	"reflect"
	"testing"

	"github.com/godbus/dbus/v5"
)

func TestPortalResult(t *testing.T) {
	ok := map[string]dbus.Variant{"uris": dbus.MakeVariant([]string{"file:///home/me/site%20a/config.yaml"})}
	if path, err := portalResult(0, ok); err != nil || path != "/home/me/site a/config.yaml" {
		t.Fatalf("%q %v", path, err)
	}
	if path, err := portalResult(1, nil); err != nil || path != "" {
		t.Fatal("cancel is not an error")
	}
	if _, err := portalResult(2, nil); err == nil {
		t.Fatal("aborted chooser")
	}
	remote := map[string]dbus.Variant{"uris": dbus.MakeVariant([]string{"sftp://host/config.yaml"})}
	if _, err := portalResult(0, remote); err == nil {
		t.Fatal("non-local file accepted")
	}
}

func TestToolArguments(t *testing.T) {
	r := Request{Save: true, Title: "另存为", Dir: "/srv/modmux", Name: "config.yaml"}
	_, args := zenity(r)
	want := []string{"--file-selection", "--title=另存为", "--file-filter=YAML | *.yaml *.yml", "--file-filter=所有文件 | *", "--save", "--confirm-overwrite", "--filename=/srv/modmux/config.yaml"}
	if !reflect.DeepEqual(args, want) {
		t.Fatalf("zenity %q", args)
	}
	_, args = kdialog(Request{Title: "打开", Dir: "/srv"})
	if !reflect.DeepEqual(args, []string{"--title", "打开", "--getopenfilename", "/srv", "YAML (*.yaml *.yml)"}) {
		t.Fatalf("kdialog %q", args)
	}
}
