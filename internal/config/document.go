// Copyright (c) 2026 Li Jinling. All rights reserved.
// This software may be modified and distributed under the terms
// of the BSD-3 Clause License. See the LICENSE file for details.

package config

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"unicode/utf8"

	"gopkg.in/yaml.v3"
)

// Revision identifies one exact content of a config file, so a writer can
// tell whether the file changed since it was read.
func Revision(content []byte) string {
	sum := sha256.Sum256(content)
	return hex.EncodeToString(sum[:8])
}

// Document is a config file as written by people. Edits are applied to the
// original text in place, so comments, blank lines, quoting and spacing that
// the YAML node tree does not round-trip stay exactly as they were.
type Document struct {
	Revision      string // of the file as read, before any Apply
	SchemaVersion int
	content       []byte
	root          yaml.Node
}

// ReadDocument reads and parses the config file at path.
func ReadDocument(path string) (*Document, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	d := &Document{Revision: Revision(content)}
	if err := d.parse(content); err != nil {
		return nil, fmt.Errorf("config: %s: %w", path, err)
	}
	var head struct {
		Version int `yaml:"version"`
	}
	if err := d.root.Decode(&head); err != nil {
		return nil, fmt.Errorf("config: 'version' must be an integer: %w", err)
	}
	d.SchemaVersion = head.Version
	return d, nil
}

func (d *Document) parse(content []byte) error {
	var root yaml.Node
	if err := yaml.Unmarshal(content, &root); err != nil {
		return fmt.Errorf("not valid YAML: %w", err)
	}
	if root.Kind != yaml.DocumentNode || len(root.Content) == 0 {
		return fmt.Errorf("empty document")
	}
	d.content, d.root = content, root
	return nil
}

// Tree returns the document as plain maps, lists and scalars, mirroring the
// file's own structure (not the normalized Config), ready for JSON encoding.
func (d *Document) Tree() (any, error) {
	var tree any
	if err := d.root.Decode(&tree); err != nil {
		return nil, err
	}
	return tree, nil
}

// Bytes returns the document text, including applied edits.
func (d *Document) Bytes() []byte {
	return d.content
}

// Edit is one change to a Document. Path walks mapping keys (strings) and
// sequence indexes (numbers) from the document root.
type Edit struct {
	Op    string `json:"op"` // "set": replace an existing single-line scalar
	Path  []any  `json:"path"`
	Value any    `json:"value"`
}

// Apply performs edits in order, each one rewriting only the text of the
// value it targets.
func (d *Document) Apply(edits []Edit) error {
	for _, e := range edits {
		if err := d.apply(e); err != nil {
			return fmt.Errorf("config: edit %v: %w", e.Path, err)
		}
	}
	return nil
}

func (d *Document) apply(e Edit) error {
	if e.Op != "set" {
		return fmt.Errorf("unsupported op %q", e.Op)
	}
	if len(e.Path) == 0 {
		return fmt.Errorf("empty path")
	}
	parent, err := d.walk(e.Path[:len(e.Path)-1])
	if err != nil {
		return err
	}
	target, err := child(parent, e.Path[len(e.Path)-1])
	if err != nil {
		return fmt.Errorf("%w (adding new fields is not supported yet)", err)
	}
	if target.Kind != yaml.ScalarNode || target.Style&(yaml.LiteralStyle|yaml.FoldedStyle) != 0 {
		return fmt.Errorf("only single-line scalar values can be set")
	}
	inFlow := parent.Style&yaml.FlowStyle != 0

	text, err := scalarText(target, e.Value, inFlow)
	if err != nil {
		return err
	}
	start, end, err := d.span(target, inFlow)
	if err != nil {
		return err
	}
	edited := append(append(append([]byte(nil), d.content[:start]...), text...), d.content[end:]...)

	// Re-parse so later edits see correct positions, and make sure the text
	// we wrote reads back as exactly the requested value.
	prev := *d
	if err := d.parse(edited); err != nil {
		*d = prev
		return fmt.Errorf("edit would break the file: %w", err)
	}
	var want, got any
	wantNode := yaml.Node{}
	if err := wantNode.Encode(e.Value); err != nil {
		*d = prev
		return err
	}
	written, _ := d.walk(e.Path)
	if written == nil || written.Decode(&got) != nil || wantNode.Decode(&want) != nil || fmt.Sprint(got) != fmt.Sprint(want) {
		*d = prev
		return fmt.Errorf("could not rewrite this value safely")
	}
	return nil
}

// scalarText renders value as YAML scalar text, keeping the old value's
// quoting style for strings.
func scalarText(old *yaml.Node, value any, inFlow bool) (string, error) {
	if str, ok := value.(string); ok {
		switch {
		case old.Style&yaml.DoubleQuotedStyle != 0:
			return strconv.Quote(str), nil
		case old.Style&yaml.SingleQuotedStyle != 0:
			return "'" + strings.ReplaceAll(str, "'", "''") + "'", nil
		}
	}
	out, err := yaml.Marshal(value)
	if err != nil {
		return "", err
	}
	text := strings.TrimSuffix(string(out), "\n")
	if strings.Contains(text, "\n") {
		return "", fmt.Errorf("value must fit on one line")
	}
	if inFlow && strings.ContainsAny(text, ",[]{}") && !strings.HasPrefix(text, `"`) {
		text = strconv.Quote(fmt.Sprint(value))
	}
	return text, nil
}

// span finds the byte range of a single-line scalar's text in d.content.
func (d *Document) span(n *yaml.Node, inFlow bool) (start, end int, err error) {
	start, err = d.offset(n.Line, n.Column)
	if err != nil {
		return 0, 0, err
	}
	c := d.content
	lineEnd := bytes.IndexByte(c[start:], '\n')
	if lineEnd < 0 {
		lineEnd = len(c)
	} else {
		lineEnd += start
	}
	switch {
	case n.Style&yaml.DoubleQuotedStyle != 0:
		for i := start + 1; i < lineEnd; i++ {
			if c[i] == '\\' {
				i++
			} else if c[i] == '"' {
				return start, i + 1, nil
			}
		}
	case n.Style&yaml.SingleQuotedStyle != 0:
		for i := start + 1; i < lineEnd; i++ {
			if c[i] == '\'' {
				if i+1 < lineEnd && c[i+1] == '\'' {
					i++
					continue
				}
				return start, i + 1, nil
			}
		}
	default:
		end = lineEnd
		for i := start; i < lineEnd; i++ {
			if (c[i] == '#' && i > start && (c[i-1] == ' ' || c[i-1] == '\t')) || (inFlow && strings.IndexByte(",]}", c[i]) >= 0) {
				end = i
				break
			}
		}
		for end > start && (c[end-1] == ' ' || c[end-1] == '\t' || c[end-1] == '\r') {
			end--
		}
		return start, end, nil
	}
	return 0, 0, fmt.Errorf("quoted value does not end on its line")
}

// offset converts a 1-based line and (rune) column to a byte offset.
func (d *Document) offset(line, column int) (int, error) {
	off := 0
	for l := 1; l < line; l++ {
		i := bytes.IndexByte(d.content[off:], '\n')
		if i < 0 {
			return 0, fmt.Errorf("line %d out of range", line)
		}
		off += i + 1
	}
	for col := 1; col < column; col++ {
		_, size := utf8.DecodeRune(d.content[off:])
		if size == 0 {
			return 0, fmt.Errorf("column %d out of range", column)
		}
		off += size
	}
	return off, nil
}

func (d *Document) walk(path []any) (*yaml.Node, error) {
	n := d.root.Content[0]
	for i, step := range path {
		next, err := child(n, step)
		if err != nil {
			return nil, fmt.Errorf("path %v: %w", path[:i+1], err)
		}
		n = next
	}
	return n, nil
}

// child returns the node under key (mapping) or index (sequence).
func child(n *yaml.Node, step any) (*yaml.Node, error) {
	switch n.Kind {
	case yaml.MappingNode:
		key, ok := step.(string)
		if !ok {
			return nil, fmt.Errorf("expected a key, got %v", step)
		}
		for i := 0; i+1 < len(n.Content); i += 2 {
			if n.Content[i].Value == key {
				return n.Content[i+1], nil
			}
		}
		return nil, fmt.Errorf("no key %q", key)
	case yaml.SequenceNode:
		i, ok := index(step)
		if !ok || i < 0 || i >= len(n.Content) {
			return nil, fmt.Errorf("no index %v", step)
		}
		return n.Content[i], nil
	}
	return nil, fmt.Errorf("cannot descend into a scalar")
}

// index accepts JSON numbers (float64) as well as Go ints.
func index(step any) (int, bool) {
	switch v := step.(type) {
	case int:
		return v, true
	case float64:
		return int(v), v == float64(int(v))
	}
	return 0, false
}

// WriteFile replaces the file at path with content atomically (write to a
// sibling temp file, then rename), keeping the original permissions.
func WriteFile(path string, content []byte) error {
	mode := os.FileMode(0o644)
	if fi, err := os.Stat(path); err == nil {
		mode = fi.Mode().Perm()
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(content); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmp.Name(), mode); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}
