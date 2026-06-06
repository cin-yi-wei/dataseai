// Package agent implements the dataseai side of the connector ↔ broker
// protocol: WebSocket endpoint, in-memory registry of live connectors,
// and an Executor that routes SQL through a connector.
//
// The wire types must stay byte-compatible with
// github.com/cin-yi-wei/dataseai-connector/pkg/protocol. Future work:
// extract to a shared module.
package agent

const (
	TypeHello     = "hello"
	TypeHelloAck  = "hello_ack"
	TypeHelloFail = "hello_fail"
	TypePing      = "ping"
	TypePong      = "pong"

	TypeQueryRequest = "query_request"
	TypeQueryMeta    = "query_meta"
	TypeQueryRows    = "query_rows"
	TypeQueryDone    = "query_done"
	TypeQueryError   = "query_error"
)

type Envelope struct {
	Type    string `json:"type"`
	Payload any    `json:"payload,omitempty"`
}

type Hello struct {
	Token        string `json:"token"`
	AgentVersion string `json:"agent_version"`
	OS           string `json:"os"`
	Arch         string `json:"arch"`
	Hostname     string `json:"hostname,omitempty"`
}

type HelloAck struct {
	AgentID          string `json:"agent_id"`
	SessionID        string `json:"session_id"`
	HeartbeatSeconds int    `json:"heartbeat_seconds"`
}

type HelloFail struct {
	Reason string `json:"reason"`
}

type Ping struct {
	Ts int64 `json:"ts"`
}

type Pong struct {
	Ts int64 `json:"ts"`
}

type MySQLTarget struct {
	Host     string `json:"host"`
	Port     int    `json:"port"`
	User     string `json:"user"`
	Password string `json:"password,omitempty"`
	Database string `json:"database,omitempty"`
}

type QueryRequest struct {
	RequestID string      `json:"request_id"`
	Target    MySQLTarget `json:"target"`
	SQL       string      `json:"sql"`
	MaxRows   int         `json:"max_rows,omitempty"`
	BatchSize int         `json:"batch_size,omitempty"`
}

type ColInfo struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

type QueryMeta struct {
	RequestID string    `json:"request_id"`
	Columns   []ColInfo `json:"columns"`
}

type QueryRows struct {
	RequestID string  `json:"request_id"`
	Rows      [][]any `json:"rows"`
}

type QueryDone struct {
	RequestID  string `json:"request_id"`
	RowCount   int    `json:"row_count"`
	DurationMs int64  `json:"duration_ms"`
}

type QueryError struct {
	RequestID string `json:"request_id"`
	Error     string `json:"error"`
}
