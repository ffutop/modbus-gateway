//go:build !darwin && !windows && !linux

package filepicker

func pick(Request) (string, error) { return "", ErrUnavailable }
