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
)

// readImport reads a .claudeq bundle sent as the request body (the dashboard
// uploads the file the user picked) and answers 200 with the task it holds —
// without queueing anything. The dashboard prefills its task sheet with the
// draft so the prompt and the paths, which come from the exporter's machine,
// can be adjusted; creating the task is the normal POST /api/tasks that
// follows. A working directory that does not exist here is reported separately
// and left out of the draft.
func (s *server) readImport(w http.ResponseWriter, r *http.Request) {
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
	draft, err := app.ReadImport(t)
	if err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, draft)
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
