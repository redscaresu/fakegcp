package handlers

import (
	"net/http"

	"github.com/go-chi/chi/v5"
)

func (app *Application) SetIAMPolicy(w http.ResponseWriter, r *http.Request) {
	project := chi.URLParam(r, "project")

	body, err := decodeBody(r)
	if err != nil {
		writeGCPError(w, http.StatusBadRequest, "Invalid JSON body", "invalid")
		return
	}

	policy, _ := body["policy"].(map[string]any)
	if policy == nil {
		writeGCPError(w, http.StatusBadRequest, "Missing required field: policy", "required")
		return
	}

	rawBindings, _ := policy["bindings"].([]any)
	if rawBindings == nil {
		rawBindings = []any{}
	}

	// Replace full project policy: clear existing roles first, then set requested bindings.
	current, err := app.repo.GetIAMPolicy(project)
	if err != nil {
		writeDomainError(w, err)
		return
	}
	currentBindings, _ := current["bindings"].([]map[string]any)
	if currentBindings == nil {
		if rawCurrent, ok := current["bindings"].([]any); ok {
			for _, it := range rawCurrent {
				if b, ok := it.(map[string]any); ok {
					role, _ := b["role"].(string)
					if role == "" {
						continue
					}
					if err := app.repo.SetIAMBinding(project, role, []string{}); err != nil {
						writeDomainError(w, err)
						return
					}
				}
			}
		}
	} else {
		for _, binding := range currentBindings {
			role, _ := binding["role"].(string)
			if role == "" {
				continue
			}
			if err := app.repo.SetIAMBinding(project, role, []string{}); err != nil {
				writeDomainError(w, err)
				return
			}
		}
	}

	for _, item := range rawBindings {
		binding, ok := item.(map[string]any)
		if !ok {
			continue
		}
		role, _ := binding["role"].(string)
		if role == "" {
			continue
		}
		rawMembers, _ := binding["members"].([]any)
		members := make([]string, 0, len(rawMembers))
		for _, m := range rawMembers {
			if s, ok := m.(string); ok && s != "" {
				members = append(members, s)
			}
		}
		if err := app.repo.SetIAMBinding(project, role, members); err != nil {
			writeDomainError(w, err)
			return
		}
	}

	updated, err := app.repo.GetIAMPolicy(project)
	if err != nil {
		writeDomainError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

func (app *Application) GetIAMPolicy(w http.ResponseWriter, r *http.Request) {
	project := chi.URLParam(r, "project")

	policy, err := app.repo.GetIAMPolicy(project)
	if err != nil {
		writeDomainError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, policy)
}
