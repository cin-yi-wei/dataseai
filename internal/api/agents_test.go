package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAgentsAPI_CreateListDelete(t *testing.T) {
	r, _ := newTestRouterWithSqliteAsMySQL(t)
	tok := registerAndLogin(t, r, "alice", "supersecret123")

	rec := post(t, r, "/api/auth/agents", map[string]any{"name": "windows"}, tok)
	if rec.Code != http.StatusOK {
		t.Fatalf("create code=%d body=%s", rec.Code, rec.Body.String())
	}
	var created struct {
		Agent struct {
			ID   int64  `json:"id"`
			Name string `json:"name"`
		} `json:"agent"`
		Token string `json:"token"`
	}
	_ = json.NewDecoder(rec.Body).Decode(&created)
	if created.Agent.ID == 0 || created.Agent.Name != "windows" {
		t.Fatalf("agent = %+v", created.Agent)
	}
	if !strings.HasPrefix(created.Token, "ag_") {
		t.Fatalf("token = %q, want ag_ prefix", created.Token)
	}

	rec = get(t, r, "/api/auth/agents", tok)
	if rec.Code != http.StatusOK {
		t.Fatalf("list code=%d body=%s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), created.Token) {
		t.Fatal("list response leaked plaintext token")
	}
	var listed struct {
		Agents []struct {
			ID   int64  `json:"id"`
			Name string `json:"name"`
		} `json:"agents"`
	}
	_ = json.NewDecoder(rec.Body).Decode(&listed)
	if len(listed.Agents) != 1 || listed.Agents[0].ID != created.Agent.ID {
		t.Fatalf("agents = %+v", listed.Agents)
	}

	rec = delete_(t, r, "/api/auth/agents/"+itoa(created.Agent.ID), tok)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("delete code=%d body=%s", rec.Code, rec.Body.String())
	}
	rec = get(t, r, "/api/auth/agents", tok)
	_ = json.NewDecoder(rec.Body).Decode(&listed)
	if len(listed.Agents) != 0 {
		t.Fatalf("agents after delete = %+v", listed.Agents)
	}
}

func TestAgentsAPI_RequiresAuth(t *testing.T) {
	r, _ := newTestRouterWithSqliteAsMySQL(t)
	rec := get(t, r, "/api/auth/agents", "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("list code = %d", rec.Code)
	}
	rec = post(t, r, "/api/auth/agents", map[string]any{"name": "windows"}, "")
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("create code = %d", rec.Code)
	}
}

func TestAgentsAPI_ListIncludesOnlineState(t *testing.T) {
	r, _ := newTestRouterWithSqliteAsMySQL(t)
	srv := httptest.NewServer(r)
	defer srv.Close()

	tok := registerAndLoginURL(t, srv.URL, "alice", "supersecret123")
	rec := postURL(t, srv.URL, "/api/auth/agents", map[string]any{"name": "windows"}, tok)
	if rec.Code != http.StatusOK {
		t.Fatalf("create code=%d body=%s", rec.Code, rec.Body.String())
	}
	var created struct {
		Agent struct {
			ID int64 `json:"id"`
		} `json:"agent"`
		Token string `json:"token"`
	}
	_ = json.NewDecoder(rec.Body).Decode(&created)

	rec = getURL(t, srv.URL, "/api/auth/agents", tok)
	var listed struct {
		Agents []struct {
			ID     int64 `json:"id"`
			Online bool  `json:"online"`
		} `json:"agents"`
	}
	_ = json.NewDecoder(rec.Body).Decode(&listed)
	if len(listed.Agents) != 1 || listed.Agents[0].ID != created.Agent.ID {
		t.Fatalf("agents = %+v", listed.Agents)
	}
	if listed.Agents[0].Online {
		t.Fatalf("online before connector login = true, want false")
	}

	c := connectTestAgent(t, srv.URL, created.Token)
	defer c.CloseNow()

	rec = getURL(t, srv.URL, "/api/auth/agents", tok)
	_ = json.NewDecoder(rec.Body).Decode(&listed)
	if len(listed.Agents) != 1 || !listed.Agents[0].Online {
		t.Fatalf("agents after connector login = %+v, want online", listed.Agents)
	}
}
