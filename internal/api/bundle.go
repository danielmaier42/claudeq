package api

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"time"

	"github.com/danielmaier42/claudeq/internal/app"
	"github.com/danielmaier42/claudeq/internal/bundle"
	"github.com/danielmaier42/claudeq/internal/task"
)

// importTask adds a task from a .claudeq bundle sent as the request body (the
// dashboard uploads the file the user picked). Responds 201 with the stored
// task — its id may differ from the file's when that id was already taken.
func (s *server) importTask(w http.ResponseWriter, r *http.Request) {
	data, err := readAllLimited(w, r, bundle.MaxSize)
	if err != nil {
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			writeErr(w, http.StatusRequestEntityTooLarge, err)
		} else {
			writeErr(w, http.StatusBadRequest, fmt.Errorf("read upload: %w", err))
		}
		return
	}
	t, err := bundle.Read(data)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	t, err = app.ImportTask(s.d.Store, t)
	if err != nil {
		// The file's task is the client's problem; a store failure is ours.
		if errors.Is(err, task.ErrInvalidTask) {
			writeErr(w, http.StatusBadRequest, err)
		} else {
			writeErr(w, http.StatusInternalServerError, err)
		}
		return
	}
	s.warmAccess(t.WorkingDir)
	writeJSON(w, http.StatusCreated, t)
}

func readAllLimited(w http.ResponseWriter, r *http.Request, limit int64) ([]byte, error) {
	var buf bytes.Buffer
	_, err := buf.ReadFrom(http.MaxBytesReader(w, r.Body, limit))
	return buf.Bytes(), err
}

// exportTask asks the user where to save the task as a .claudeq file (native
// "Save as" panel) and writes it there. 200 {"path"} on success, 204 when the
// user cancelled, 503 when no dialog is available (headless daemon).
func (s *server) exportTask(w http.ResponseWriter, r *http.Request) {
	if s.d.SaveFile == nil {
		writeErr(w, http.StatusServiceUnavailable, errors.New("save dialog not available"))
		return
	}
	var buf bytes.Buffer
	t, err := app.ExportTask(s.d.Store, r.PathValue("id"), &buf, time.Now())
	if err != nil {
		if errors.Is(err, app.ErrNotFound) {
			writeErr(w, http.StatusNotFound, err)
		} else {
			writeErr(w, http.StatusInternalServerError, err)
		}
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Minute)
	defer cancel()
	path, chosen, err := s.d.SaveFile(ctx, "Export task “"+t.Name+"”", bundle.FileName(t))
	if err != nil {
		writeErr(w, http.StatusBadGateway, err)
		return
	}
	if !chosen {
		w.WriteHeader(http.StatusNoContent)
		return
	}
	// The panel already confirmed replacing the file it showed. If the
	// extension had to be appended, the resulting name was never shown, so an
	// existing file there is not overwritten — the user picks it explicitly.
	path, appended := bundle.EnsureExt(path)
	if err := bundle.Save(path, buf.Bytes(), !appended); err != nil {
		if errors.Is(err, fs.ErrExist) {
			err = fmt.Errorf("%s already exists; choose that file in the save panel to replace it", path)
		}
		writeErr(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"path": path})
}
