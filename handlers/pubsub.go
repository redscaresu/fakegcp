package handlers

import (
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
)

func (app *Application) CreateTopic(w http.ResponseWriter, r *http.Request) {
	project := chi.URLParam(r, "project")
	topic := chi.URLParam(r, "topic")

	body, err := decodeBody(r)
	if err != nil {
		writeGCPError(w, http.StatusBadRequest, "Invalid JSON body", "invalid")
		return
	}
	body["name"] = fmt.Sprintf("projects/%s/topics/%s", project, topic)

	created, err := app.repo.CreateTopic(project, body)
	if err != nil {
		writeCreateError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, created)
}

func (app *Application) ListTopics(w http.ResponseWriter, r *http.Request) {
	project := chi.URLParam(r, "project")

	items, err := app.repo.ListTopics(project)
	if err != nil {
		writeDomainError(w, err)
		return
	}
	if items == nil {
		items = []map[string]any{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"topics": items})
}

func (app *Application) GetTopic(w http.ResponseWriter, r *http.Request) {
	project := chi.URLParam(r, "project")
	topic := chi.URLParam(r, "topic")

	item, err := app.repo.GetTopic(project, topic)
	if err != nil {
		writeDomainError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (app *Application) DeleteTopic(w http.ResponseWriter, r *http.Request) {
	project := chi.URLParam(r, "project")
	topic := chi.URLParam(r, "topic")

	if err := app.repo.DeleteTopic(project, topic); err != nil {
		writeDomainError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{})
}

func (app *Application) CreateSubscription(w http.ResponseWriter, r *http.Request) {
	project := chi.URLParam(r, "project")
	subscription := chi.URLParam(r, "subscription")

	body, err := decodeBody(r)
	if err != nil {
		writeGCPError(w, http.StatusBadRequest, "Invalid JSON body", "invalid")
		return
	}
	body["name"] = fmt.Sprintf("projects/%s/subscriptions/%s", project, subscription)

	created, err := app.repo.CreateSubscription(project, body)
	if err != nil {
		writeCreateError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, created)
}

func (app *Application) ListSubscriptions(w http.ResponseWriter, r *http.Request) {
	project := chi.URLParam(r, "project")

	items, err := app.repo.ListSubscriptions(project)
	if err != nil {
		writeDomainError(w, err)
		return
	}
	if items == nil {
		items = []map[string]any{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"subscriptions": items})
}

func (app *Application) GetSubscription(w http.ResponseWriter, r *http.Request) {
	project := chi.URLParam(r, "project")
	subscription := chi.URLParam(r, "subscription")

	item, err := app.repo.GetSubscription(project, subscription)
	if err != nil {
		writeDomainError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (app *Application) UpdateSubscription(w http.ResponseWriter, r *http.Request) {
	project := chi.URLParam(r, "project")
	subscription := chi.URLParam(r, "subscription")

	patch, err := decodeBody(r)
	if err != nil {
		writeGCPError(w, http.StatusBadRequest, "Invalid JSON body", "invalid")
		return
	}
	updated, err := app.repo.UpdateSubscription(project, subscription, patch)
	if err != nil {
		writeDomainError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

func (app *Application) DeleteSubscription(w http.ResponseWriter, r *http.Request) {
	project := chi.URLParam(r, "project")
	subscription := chi.URLParam(r, "subscription")

	if err := app.repo.DeleteSubscription(project, subscription); err != nil {
		writeDomainError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{})
}
