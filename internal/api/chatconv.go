package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/conray/dataseai/internal/auth"
	"github.com/conray/dataseai/internal/store"
	"github.com/go-chi/chi/v5"
)

func convIDParam(r *http.Request) (int64, error) {
	return strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
}

// GET /api/chat/conversations?conn_id=&db=
func handleListConversations(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		u, _ := auth.UserFromContext(r.Context())
		connID, _ := strconv.ParseInt(r.URL.Query().Get("conn_id"), 10, 64)
		dbName := r.URL.Query().Get("db")
		list, err := d.Store.ListConversations(u.ID, connID, dbName)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "list failed")
			return
		}
		if list == nil {
			list = []store.Conversation{}
		}
		writeJSON(w, http.StatusOK, map[string]any{"conversations": list})
	}
}

type createConversationReq struct {
	ConnID int64  `json:"conn_id"`
	DB     string `json:"db"`
	Name   string `json:"name"`
}

// POST /api/chat/conversations
func handleCreateConversation(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		u, _ := auth.UserFromContext(r.Context())
		var req createConversationReq
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid json")
			return
		}
		req.Name = strings.TrimSpace(req.Name)
		if req.Name == "" {
			req.Name = "未命名"
		}
		c, err := d.Store.CreateConversation(u.ID, req.ConnID, req.DB, req.Name, time.Now())
		if err != nil {
			writeError(w, http.StatusInternalServerError, "create failed")
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"conversation": c})
	}
}

type renameConversationReq struct {
	Name string `json:"name"`
}

// PUT /api/chat/conversations/{id}
func handleRenameConversation(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		u, _ := auth.UserFromContext(r.Context())
		id, err := convIDParam(r)
		if err != nil {
			writeError(w, http.StatusBadRequest, "bad id")
			return
		}
		var req renameConversationReq
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid json")
			return
		}
		req.Name = strings.TrimSpace(req.Name)
		if req.Name == "" {
			writeError(w, http.StatusBadRequest, "name required")
			return
		}
		if err := d.Store.RenameConversation(u.ID, id, req.Name); err != nil {
			if errors.Is(err, store.ErrNotFound) {
				writeError(w, http.StatusNotFound, "not found")
				return
			}
			writeError(w, http.StatusInternalServerError, "rename failed")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// DELETE /api/chat/conversations/{id}
func handleDeleteConversation(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		u, _ := auth.UserFromContext(r.Context())
		id, err := convIDParam(r)
		if err != nil {
			writeError(w, http.StatusBadRequest, "bad id")
			return
		}
		if err := d.Store.DeleteConversation(u.ID, id); err != nil {
			if errors.Is(err, store.ErrNotFound) {
				writeError(w, http.StatusNotFound, "not found")
				return
			}
			writeError(w, http.StatusInternalServerError, "delete failed")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// GET /api/chat/conversations/{id}/messages
func handleGetConversationMessages(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		u, _ := auth.UserFromContext(r.Context())
		id, err := convIDParam(r)
		if err != nil {
			writeError(w, http.StatusBadRequest, "bad id")
			return
		}
		msgs, err := d.Store.GetMessages(u.ID, id)
		if err != nil {
			if errors.Is(err, store.ErrNotFound) {
				writeError(w, http.StatusNotFound, "not found")
				return
			}
			writeError(w, http.StatusInternalServerError, "load failed")
			return
		}
		if msgs == nil {
			msgs = []store.StoredChatMessage{}
		}
		writeJSON(w, http.StatusOK, map[string]any{"messages": msgs})
	}
}

type saveMessagesReq struct {
	Messages []store.StoredChatMessage `json:"messages"`
}

// PUT /api/chat/conversations/{id}/messages
func handleSaveConversationMessages(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		u, _ := auth.UserFromContext(r.Context())
		id, err := convIDParam(r)
		if err != nil {
			writeError(w, http.StatusBadRequest, "bad id")
			return
		}
		var req saveMessagesReq
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid json")
			return
		}
		if err := d.Store.ReplaceMessages(u.ID, id, req.Messages, time.Now()); err != nil {
			if errors.Is(err, store.ErrNotFound) {
				writeError(w, http.StatusNotFound, "not found")
				return
			}
			writeError(w, http.StatusInternalServerError, "save failed")
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}
