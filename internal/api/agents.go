package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/conray/dataseai/internal/auth"
	"github.com/conray/dataseai/internal/store"
	"github.com/go-chi/chi/v5"
)

type agentCreateReq struct {
	Name string `json:"name"`
}

func handleCreateAgent(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		u, _ := auth.UserFromContext(r.Context())
		var req agentCreateReq
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid json")
			return
		}
		a, token, err := d.Store.CreateAgent(u.ID, req.Name)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"agent": a, "token": token})
	}
}

func handleListAgents(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		u, _ := auth.UserFromContext(r.Context())
		agents, err := d.Store.ListAgents(u.ID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "list agents failed")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"agents": agents})
	}
}

func handleDeleteAgent(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		u, _ := auth.UserFromContext(r.Context())
		id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
		if err != nil {
			writeError(w, http.StatusBadRequest, "bad id")
			return
		}
		if err := d.Store.DeleteAgent(id, u.ID); err != nil {
			if errors.Is(err, store.ErrAgentNotFound) {
				writeError(w, http.StatusNotFound, "agent not found")
				return
			}
			writeError(w, http.StatusInternalServerError, "delete agent failed")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}
