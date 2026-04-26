package handlers

import (
	"net/http"

	"github.com/go-chi/chi/v5"
)

func (app *Application) ResetState(w http.ResponseWriter, r *http.Request) {
	if err := app.repo.Reset(); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"status": "error", "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok"})
}

func (app *Application) SnapshotState(w http.ResponseWriter, r *http.Request) {
	if err := app.repo.Snapshot(); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"status": "error", "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok"})
}

func (app *Application) RestoreState(w http.ResponseWriter, r *http.Request) {
	if err := app.repo.Restore(); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"status": "error", "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok"})
}

func (app *Application) FullState(w http.ResponseWriter, r *http.Request) {
	state, err := app.repo.FullState()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"status": "error", "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, state)
}

func (app *Application) ServiceState(w http.ResponseWriter, r *http.Request) {
	service := chi.URLParam(r, "service")
	state, err := app.repo.ServiceState(service)
	if err != nil {
		writeDomainError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, state)
}
