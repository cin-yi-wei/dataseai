package api

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/conray/dataseai/internal/crypto"
	"github.com/conray/dataseai/internal/store"
	_ "github.com/mattn/go-sqlite3"
)

// newAIPolicyRouter returns a router with Pool=nil so that listAllTablesForAIPolicy
// skips the MySQL call and returns (nil, nil) — safe for unit tests without a real DB.
func newAIPolicyRouter(t *testing.T) (http.Handler, *store.Store, *crypto.Cipher) {
	t.Helper()
	db, _ := sql.Open("sqlite3", ":memory:")
	t.Cleanup(func() { db.Close() })
	if err := store.Migrate(db); err != nil {
		t.Fatal(err)
	}
	s := &store.Store{DB: db}
	c := newCipher(t)
	// Pool intentionally nil: listAllTablesForAIPolicy returns (nil,nil) → no 502
	r := NewRouter(Deps{Version: "test", Store: s, Cipher: c, Pool: nil, Registration: "open"})
	return r, s, c
}

// TestAIWritesMasterToggle — GET defaults false, PUT enabled=true, GET returns true.
func TestAIWritesMasterToggle(t *testing.T) {
	r, _, _ := newAIPolicyRouter(t)
	tok := registerAndLogin(t, r, "alice", "supersecret123")

	// GET: should default to false
	rec := get(t, r, "/api/auth/ai-writes", tok)
	if rec.Code != 200 {
		t.Fatalf("GET ai-writes code = %d body=%s", rec.Code, rec.Body.String())
	}
	var resp aiWritesResp
	_ = json.NewDecoder(rec.Body).Decode(&resp)
	if resp.Enabled {
		t.Fatal("expected enabled=false by default")
	}

	// PUT enabled=true
	rec = putJSON(t, r, "/api/auth/ai-writes", map[string]any{"enabled": true}, tok)
	if rec.Code != 200 {
		t.Fatalf("PUT ai-writes code = %d body=%s", rec.Code, rec.Body.String())
	}
	var putResp aiWritesResp
	_ = json.NewDecoder(rec.Body).Decode(&putResp)
	if !putResp.Enabled {
		t.Fatal("PUT response should have enabled=true")
	}

	// GET: should now return true
	rec = get(t, r, "/api/auth/ai-writes", tok)
	_ = json.NewDecoder(rec.Body).Decode(&resp)
	if !resp.Enabled {
		t.Fatal("expected enabled=true after PUT")
	}
}

// TestAIPolicyUpsertAndList — PUT policy for table "t1", GET returns it in configured.
func TestAIPolicyUpsertAndList(t *testing.T) {
	r, _, _ := newAIPolicyRouter(t)
	tok := registerAndLogin(t, r, "alice", "supersecret123")
	connID := seedDMLConn(t, r, tok)

	// PUT policy for t1
	rec := putJSON(t, r, "/api/auth/ai-policy", map[string]any{
		"conn":  connID,
		"db":    "db1",
		"table": "t1",
		"policy": map[string]any{
			"insert": true, "update": false, "delete": false, "ddl": false,
		},
	}, tok)
	if rec.Code != 200 {
		t.Fatalf("PUT ai-policy code = %d body=%s", rec.Code, rec.Body.String())
	}
	var putResp map[string]any
	_ = json.NewDecoder(rec.Body).Decode(&putResp)
	if putResp["table"] != "t1" {
		t.Fatalf("PUT response table = %v", putResp["table"])
	}

	// GET: Pool is nil, so unconfigured will be nil/empty — that's fine.
	// We just assert configured has t1.
	rec = get(t, r, "/api/auth/ai-policy?conn="+itoa(connID)+"&db=db1", tok)
	if rec.Code != 200 {
		t.Fatalf("GET ai-policy code = %d body=%s", rec.Code, rec.Body.String())
	}
	var listResp struct {
		Configured   []store.AITablePolicy `json:"configured"`
		Unconfigured []string              `json:"unconfigured"`
	}
	_ = json.NewDecoder(rec.Body).Decode(&listResp)
	if len(listResp.Configured) != 1 || listResp.Configured[0].Table != "t1" {
		t.Fatalf("expected t1 in configured, got %+v", listResp.Configured)
	}
	if !listResp.Configured[0].Policy.Insert {
		t.Fatalf("t1 policy.Insert should be true")
	}
}

// TestAIPolicyBatch — PUT batch with 3 tables, response says updated=3.
func TestAIPolicyBatch(t *testing.T) {
	r, _, _ := newAIPolicyRouter(t)
	tok := registerAndLogin(t, r, "alice", "supersecret123")
	connID := seedDMLConn(t, r, tok)

	rec := putJSON(t, r, "/api/auth/ai-policy/batch", map[string]any{
		"conn":   connID,
		"db":     "db1",
		"tables": []string{"t1", "t2", "t3"},
		"policy": map[string]any{
			"insert": true, "update": true, "delete": false, "ddl": false,
		},
	}, tok)
	if rec.Code != 200 {
		t.Fatalf("PUT batch code = %d body=%s", rec.Code, rec.Body.String())
	}
	var resp map[string]any
	_ = json.NewDecoder(rec.Body).Decode(&resp)
	// JSON numbers decode as float64
	if resp["updated"] != float64(3) {
		t.Fatalf("expected updated=3, got %v", resp["updated"])
	}
}

// TestAIPolicyDelete — PUT a policy then DELETE, verify the row is gone via GET.
func TestAIPolicyDelete(t *testing.T) {
	r, _, _ := newAIPolicyRouter(t)
	tok := registerAndLogin(t, r, "alice", "supersecret123")
	connID := seedDMLConn(t, r, tok)

	// PUT a policy for t1
	rec := putJSON(t, r, "/api/auth/ai-policy", map[string]any{
		"conn":  connID,
		"db":    "db1",
		"table": "t1",
		"policy": map[string]any{
			"insert": true, "update": false, "delete": false, "ddl": false,
		},
	}, tok)
	if rec.Code != 200 {
		t.Fatalf("PUT ai-policy code = %d", rec.Code)
	}

	// Verify t1 is in configured
	rec = get(t, r, "/api/auth/ai-policy?conn="+itoa(connID)+"&db=db1", tok)
	var before struct {
		Configured []store.AITablePolicy `json:"configured"`
	}
	_ = json.NewDecoder(rec.Body).Decode(&before)
	if len(before.Configured) != 1 {
		t.Fatalf("expected 1 configured before delete, got %d", len(before.Configured))
	}

	// DELETE the policy (query params encoded in path)
	path := "/api/auth/ai-policy?conn=" + itoa(connID) + "&db=db1&table=t1"
	rec = delete_(t, r, path, tok)
	if rec.Code != 200 {
		t.Fatalf("DELETE ai-policy code = %d body=%s", rec.Code, rec.Body.String())
	}
	var delResp map[string]any
	_ = json.NewDecoder(rec.Body).Decode(&delResp)
	if delResp["ok"] != true {
		t.Fatalf("expected ok=true, got %v", delResp)
	}

	// Verify t1 is no longer in configured
	rec = get(t, r, "/api/auth/ai-policy?conn="+itoa(connID)+"&db=db1", tok)
	var after struct {
		Configured []store.AITablePolicy `json:"configured"`
	}
	_ = json.NewDecoder(rec.Body).Decode(&after)
	if len(after.Configured) != 0 {
		t.Fatalf("expected 0 configured after delete, got %d", len(after.Configured))
	}
}
