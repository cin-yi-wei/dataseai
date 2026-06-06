package agent

import (
	"context"
	"testing"
	"time"

	"github.com/conray/dataseai/internal/mysql"
)

type fakeAgentConn struct {
	sent    chan Envelope
	waiters map[string]chan Envelope
}

func newFakeAgentConn() *fakeAgentConn {
	return &fakeAgentConn{
		sent:    make(chan Envelope, 1),
		waiters: map[string]chan Envelope{},
	}
}

func (f *fakeAgentConn) Send(ctx context.Context, env Envelope) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case f.sent <- env:
		return nil
	}
}

func (f *fakeAgentConn) Subscribe(requestID string) (<-chan Envelope, func()) {
	ch := make(chan Envelope, 8)
	f.waiters[requestID] = ch
	return ch, func() {
		delete(f.waiters, requestID)
		close(ch)
	}
}

func (f *fakeAgentConn) waitForQuery(t *testing.T) QueryRequest {
	t.Helper()
	select {
	case env := <-f.sent:
		if env.Type != TypeQueryRequest {
			t.Fatalf("sent type = %q, want %q", env.Type, TypeQueryRequest)
		}
		var req QueryRequest
		if err := remarshal(env.Payload, &req); err != nil {
			t.Fatal(err)
		}
		return req
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for query request")
		return QueryRequest{}
	}
}

func (f *fakeAgentConn) deliver(env Envelope) {
	requestID := extractRequestID(env)
	if requestID == "" {
		switch msg := env.Payload.(type) {
		case QueryMeta:
			requestID = msg.RequestID
		case QueryRows:
			requestID = msg.RequestID
		case QueryDone:
			requestID = msg.RequestID
		case QueryError:
			requestID = msg.RequestID
		}
	}
	f.waiters[requestID] <- env
}

func TestAgentExecutorRun_CollectsMetaRowsAndDone(t *testing.T) {
	fc := newFakeAgentConn()
	exec := AgentExecutor{
		Conn: fc,
		Target: MySQLTarget{
			Host: "127.0.0.1", Port: 3306, User: "root", Password: "pw", Database: "mysql",
		},
	}

	go func() {
		req := fc.waitForQuery(t)
		fc.deliver(Envelope{Type: TypeQueryMeta, Payload: QueryMeta{
			RequestID: req.RequestID,
			Columns:   []ColInfo{{Name: "v", Type: "VARCHAR"}, {Name: "u", Type: "VARCHAR"}},
		}})
		fc.deliver(Envelope{Type: TypeQueryRows, Payload: QueryRows{
			RequestID: req.RequestID,
			Rows:      [][]any{{"8.0.46", "root@localhost"}},
		}})
		fc.deliver(Envelope{Type: TypeQueryDone, Payload: QueryDone{
			RequestID: req.RequestID,
			RowCount:  1,
		}})
	}()

	out, err := exec.Run(context.Background(), "SELECT version() AS v, user() AS u", mysql.RunOpts{MaxRows: 100})
	if err != nil {
		t.Fatal(err)
	}
	if out.Kind != mysql.StmtSelect {
		t.Fatalf("kind = %v, want select", out.Kind)
	}
	if len(out.Columns) != 2 || out.Columns[0] != "v" || out.Columns[1] != "u" {
		t.Fatalf("columns = %#v", out.Columns)
	}
	if len(out.Rows) != 1 || out.Rows[0][0] != "8.0.46" || out.Rows[0][1] != "root@localhost" {
		t.Fatalf("rows = %#v", out.Rows)
	}
}

func TestAgentExecutorRun_IncludesSSHTarget(t *testing.T) {
	fc := newFakeAgentConn()
	exec := AgentExecutor{
		Conn: fc,
		Target: MySQLTarget{
			Host: "10.0.2.15", Port: 3306, User: "app", Password: "dbpw", Database: "appdb",
			SSH: &SSHConfig{
				Host: "bastion.example.com", Port: 22, User: "ubuntu", Password: "sshpw",
			},
		},
	}

	go func() {
		req := fc.waitForQuery(t)
		if req.Target.SSH == nil {
			t.Fatal("target ssh config is nil")
		}
		if req.Target.SSH.Host != "bastion.example.com" || req.Target.SSH.User != "ubuntu" || req.Target.SSH.Password != "sshpw" {
			t.Fatalf("ssh = %+v", req.Target.SSH)
		}
		fc.deliver(Envelope{Type: TypeQueryMeta, Payload: QueryMeta{RequestID: req.RequestID}})
		fc.deliver(Envelope{Type: TypeQueryDone, Payload: QueryDone{RequestID: req.RequestID}})
	}()

	if _, err := exec.Run(context.Background(), "SELECT 1", mysql.RunOpts{}); err != nil {
		t.Fatal(err)
	}
}

func TestAgentExecutorRun_QueryErrorReturnsError(t *testing.T) {
	fc := newFakeAgentConn()
	exec := AgentExecutor{Conn: fc, Target: MySQLTarget{Host: "127.0.0.1", Port: 3306, User: "root"}}

	go func() {
		req := fc.waitForQuery(t)
		fc.deliver(Envelope{Type: TypeQueryError, Payload: QueryError{
			RequestID: req.RequestID,
			Error:     "access denied",
		}})
	}()

	_, err := exec.Run(context.Background(), "SELECT 1", mysql.RunOpts{})
	if err == nil || err.Error() != "access denied" {
		t.Fatalf("err = %v, want access denied", err)
	}
}
