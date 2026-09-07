// Package bundle reads and writes the portable ".claudeq" task file: a zip
// archive holding the task's settings as JSON (task.json) and its prompt as
// Markdown (prompt.md). It is the exchange format for sharing a task with a
// colleague — the whole task goes in, and the importer gets it back as-is,
// prompt included, ready to be adjusted with the normal edit flow.
package bundle

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/danielmaier42/claudeq/internal/task"
)

const (
	// Ext is the file extension of a task bundle.
	Ext = ".claudeq"
	// Format identifies a bundle's task.json; a future incompatible layout
	// bumps Version.
	Format = "claudeq-task"
	// Version is the task.json layout version this package writes and reads.
	Version = 1

	settingsName = "task.json"
	promptName   = "prompt.md"

	// MaxSize bounds a bundle and each of its entries, compressed or not. A
	// task is a few kilobytes of settings plus a prompt that is at most pages
	// long, so anything larger is not a task file.
	MaxSize = 4 << 20
)

// ErrInvalid is the base error for a file that is not a usable task bundle.
var ErrInvalid = errors.New("invalid .claudeq file")

// envelope is the task.json layout. Task carries every task field except the
// prompt, which lives in prompt.md so it stays readable and editable as text.
type envelope struct {
	Format     string    `json:"format"`
	Version    int       `json:"format_version"`
	ExportedAt time.Time `json:"exported_at"`
	Task       settings  `json:"task"`
}

// settings is a task without its prompt: the outer Prompt field takes the
// "prompt" JSON key away from the embedded one, and being always empty it is
// omitted — while every other task field, present and future, round-trips
// untouched.
type settings struct {
	task.Task
	Prompt string `json:"prompt,omitempty"`
}

// Write serialises t as a task bundle to w. now is recorded as the export time.
func Write(w io.Writer, t task.Task, now time.Time) error {
	env := envelope{Format: Format, Version: Version, ExportedAt: now.UTC().Truncate(time.Second)}
	env.Task.Task = t
	meta, err := json.MarshalIndent(env, "", "  ")
	if err != nil {
		return fmt.Errorf("encode task settings: %w", err)
	}
	meta = append(meta, '\n')

	zw := zip.NewWriter(w)
	for _, f := range []struct {
		name string
		data []byte
	}{{settingsName, meta}, {promptName, []byte(t.Prompt)}} {
		fw, err := zw.CreateHeader(&zip.FileHeader{Name: f.name, Method: zip.Deflate, Modified: env.ExportedAt})
		if err != nil {
			return fmt.Errorf("add %s: %w", f.name, err)
		}
		if _, err := fw.Write(f.data); err != nil {
			return fmt.Errorf("write %s: %w", f.name, err)
		}
	}
	if err := zw.Close(); err != nil {
		return fmt.Errorf("finish bundle: %w", err)
	}
	return nil
}

// Read parses a task bundle. It checks the file layout — a zip with task.json
// in the known format and a non-empty prompt.md — and nothing about the task
// itself: no id, name or enabled-state is invented, and the importer decides
// how to fit the task into its queue.
func Read(data []byte) (task.Task, error) {
	if len(data) > MaxSize {
		return task.Task{}, fmt.Errorf("%w: %d bytes exceeds the %d byte limit", ErrInvalid, len(data), MaxSize)
	}
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return task.Task{}, fmt.Errorf("%w: not a zip archive: %w", ErrInvalid, err)
	}
	meta, err := entry(zr, settingsName)
	if err != nil {
		return task.Task{}, err
	}
	prompt, err := entry(zr, promptName)
	if err != nil {
		return task.Task{}, err
	}

	var env envelope
	if err := json.Unmarshal(meta, &env); err != nil {
		return task.Task{}, fmt.Errorf("%w: %s: %w", ErrInvalid, settingsName, err)
	}
	if env.Format != Format {
		return task.Task{}, fmt.Errorf("%w: %s has format %q, want %q", ErrInvalid, settingsName, env.Format, Format)
	}
	if env.Version != Version {
		return task.Task{}, fmt.Errorf("%w: %s has format_version %d, this claudeq reads version %d",
			ErrInvalid, settingsName, env.Version, Version)
	}
	if len(prompt) == 0 {
		return task.Task{}, fmt.Errorf("%w: %s is empty", ErrInvalid, promptName)
	}
	t := env.Task.Task
	t.Prompt = string(prompt)
	return t, nil
}

// entry returns the content of the named file. Entries are expected at the
// archive root, but a bundle that was unpacked, edited and re-zipped by hand
// often carries a folder around them (Finder's Compress does that) — one
// folder level is accepted, and macOS resource-fork clutter is ignored.
func entry(zr *zip.Reader, name string) ([]byte, error) {
	var found *zip.File
	for _, f := range zr.File {
		dir, base := path.Split(f.Name)
		if base != name || strings.HasPrefix(dir, "__MACOSX/") || strings.Count(dir, "/") > 1 {
			continue
		}
		if found == nil || dir == "" {
			found = f
		}
	}
	if found == nil {
		return nil, fmt.Errorf("%w: missing %s", ErrInvalid, name)
	}
	if found.UncompressedSize64 > MaxSize {
		return nil, fmt.Errorf("%w: %s exceeds %d bytes", ErrInvalid, name, MaxSize)
	}
	rc, err := found.Open()
	if err != nil {
		return nil, fmt.Errorf("%w: open %s: %w", ErrInvalid, name, err)
	}
	defer func() { _ = rc.Close() }()
	// The header's size is attacker-controlled, so the limit is enforced on
	// the inflated bytes as well.
	data, err := io.ReadAll(io.LimitReader(rc, MaxSize+1))
	if err != nil {
		return nil, fmt.Errorf("%w: read %s: %w", ErrInvalid, name, err)
	}
	if len(data) > MaxSize {
		return nil, fmt.Errorf("%w: %s exceeds %d bytes", ErrInvalid, name, MaxSize)
	}
	return data, nil
}

// FileName is the default file name for an exported task: its id plus Ext,
// with path separators replaced so the name is always a plain file name.
func FileName(t task.Task) string {
	base := strings.TrimSpace(t.ID)
	if base == "" {
		base = "task"
	}
	base = strings.NewReplacer("/", "-", "\\", "-", ":", "-").Replace(base)
	return base + Ext
}

// EnsureExt returns p with the bundle extension, and whether it had to be
// appended (the extension check is case-insensitive).
func EnsureExt(p string) (string, bool) {
	if strings.EqualFold(filepath.Ext(p), Ext) {
		return p, false
	}
	return p + Ext, true
}

// Save writes a bundle's bytes to p. Unless overwrite is set, an existing
// file is left alone and the error wraps fs.ErrExist.
func Save(p string, data []byte, overwrite bool) error {
	mode := os.O_WRONLY | os.O_CREATE | os.O_EXCL
	if overwrite {
		mode = os.O_WRONLY | os.O_CREATE | os.O_TRUNC
	}
	f, err := os.OpenFile(p, mode, 0o644)
	if err != nil {
		return fmt.Errorf("save %s: %w", p, err)
	}
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		return fmt.Errorf("save %s: %w", p, err)
	}
	if err := f.Close(); err != nil {
		return fmt.Errorf("save %s: %w", p, err)
	}
	return nil
}

// Load reads a bundle file from disk, refusing anything larger than MaxSize
// before reading it (a mis-picked file is not read into memory first).
func Load(p string) (task.Task, error) {
	fi, err := os.Stat(p)
	if err != nil {
		return task.Task{}, fmt.Errorf("read %s: %w", p, err)
	}
	if fi.Size() > MaxSize {
		return task.Task{}, fmt.Errorf("%s: %w: %d bytes exceeds the %d byte limit", p, ErrInvalid, fi.Size(), MaxSize)
	}
	data, err := os.ReadFile(p)
	if err != nil {
		return task.Task{}, fmt.Errorf("read %s: %w", p, err)
	}
	t, err := Read(data)
	if err != nil {
		return task.Task{}, fmt.Errorf("%s: %w", p, err)
	}
	return t, nil
}
