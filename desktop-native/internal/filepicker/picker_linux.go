package filepicker

import (
	"errors"
	"fmt"
	"net/url"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"

	"github.com/godbus/dbus/v5"
)

// pick prefers the XDG desktop portal, which shows the desktop's own chooser
// (GNOME, KDE and others, also inside sandboxes), then zenity and kdialog.
func pick(r Request) (string, error) {
	path, err := portal(r)
	if !errors.Is(err, ErrUnavailable) {
		return path, err
	}
	for _, tool := range []func(Request) (string, []string){zenity, kdialog} {
		name, args := tool(r)
		if _, err := exec.LookPath(name); err != nil {
			continue
		}
		out, err := exec.Command(name, args...).Output()
		var exit *exec.ExitError
		if errors.As(err, &exit) && exit.ExitCode() == 1 {
			return "", nil // cancelled
		}
		if err != nil {
			return "", fmt.Errorf("%s：%w", name, err)
		}
		return strings.TrimRight(string(out), "\r\n"), nil
	}
	return "", ErrUnavailable
}

func zenity(r Request) (string, []string) {
	args := []string{"--file-selection", "--title=" + r.Title,
		"--file-filter=YAML | " + strings.Join(Patterns, " "), "--file-filter=所有文件 | *"}
	start := r.Dir + string(filepath.Separator)
	if r.Save {
		args = append(args, "--save", "--confirm-overwrite")
		start = filepath.Join(r.Dir, r.Name)
	}
	return "zenity", append(args, "--filename="+start)
}

func kdialog(r Request) (string, []string) {
	mode, start := "--getopenfilename", r.Dir
	if r.Save {
		mode, start = "--getsavefilename", filepath.Join(r.Dir, r.Name)
	}
	return "kdialog", []string{"--title", r.Title, mode, start, "YAML (" + strings.Join(Patterns, " ") + ")"}
}

const (
	portalName = "org.freedesktop.portal.Desktop"
	portalPath = "/org/freedesktop/portal/desktop"
)

type portalFilter struct {
	Name  string
	Rules []portalRule
}

type portalRule struct {
	Kind    uint32 // 0: glob pattern
	Pattern string
}

var portalRequests atomic.Uint64

// portal calls org.freedesktop.portal.FileChooser and waits for the
// Response signal on the request object it predicts from handle_token.
func portal(r Request) (string, error) {
	conn, err := dbus.SessionBus()
	if err != nil {
		return "", ErrUnavailable
	}
	names := conn.Names()
	if len(names) == 0 {
		return "", ErrUnavailable
	}
	token := fmt.Sprintf("modmux%d", portalRequests.Add(1))
	sender := strings.ReplaceAll(strings.TrimPrefix(names[0], ":"), ".", "_")
	request := dbus.ObjectPath(portalPath + "/request/" + sender + "/" + token)
	match := []dbus.MatchOption{dbus.WithMatchObjectPath(request), dbus.WithMatchInterface("org.freedesktop.portal.Request"), dbus.WithMatchMember("Response")}
	if err := conn.AddMatchSignal(match...); err != nil {
		return "", ErrUnavailable
	}
	defer conn.RemoveMatchSignal(match...)
	signals := make(chan *dbus.Signal, 4)
	conn.Signal(signals)
	defer conn.RemoveSignal(signals)

	var patterns []portalRule
	for _, p := range Patterns {
		patterns = append(patterns, portalRule{Pattern: p})
	}
	yaml := portalFilter{Name: "YAML", Rules: patterns}
	options := map[string]dbus.Variant{
		"handle_token":   dbus.MakeVariant(token),
		"modal":          dbus.MakeVariant(true),
		"filters":        dbus.MakeVariant([]portalFilter{yaml, {Name: "所有文件", Rules: []portalRule{{Pattern: "*"}}}}),
		"current_filter": dbus.MakeVariant(yaml),
	}
	if r.Dir != "" {
		options["current_folder"] = dbus.MakeVariant(append([]byte(r.Dir), 0))
	}
	method := "org.freedesktop.portal.FileChooser.OpenFile"
	if r.Save {
		method = "org.freedesktop.portal.FileChooser.SaveFile"
		options["current_name"] = dbus.MakeVariant(r.Name)
	}
	var handle dbus.ObjectPath
	if err := conn.Object(portalName, portalPath).Call(method, 0, "", r.Title, options).Store(&handle); err != nil {
		return "", ErrUnavailable // no portal, or no FileChooser backend
	}
	for s := range signals {
		if (s.Path != request && s.Path != handle) || s.Name != "org.freedesktop.portal.Request.Response" || len(s.Body) < 2 {
			continue
		}
		code, _ := s.Body[0].(uint32)
		results, _ := s.Body[1].(map[string]dbus.Variant)
		return portalResult(code, results)
	}
	return "", errors.New("文件选择器连接已断开")
}

// portalResult reads the first chosen file URI; code 1 is a cancellation.
func portalResult(code uint32, results map[string]dbus.Variant) (string, error) {
	switch code {
	case 0:
	case 1:
		return "", nil
	default:
		return "", errors.New("文件选择器已中止")
	}
	var uris []string
	if v, ok := results["uris"]; ok {
		_ = v.Store(&uris)
	}
	if len(uris) == 0 {
		return "", nil
	}
	u, err := url.Parse(uris[0])
	if err != nil || u.Scheme != "file" {
		return "", fmt.Errorf("不支持的文件位置：%s", uris[0])
	}
	return u.Path, nil
}
