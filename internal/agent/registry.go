package agent

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"sync"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
)

// Conn is the live WebSocket session a connector has with this broker. It
// owns the write mutex (so heartbeat + query-routing goroutines don't race)
// and exposes the routing surface that AgentExecutor uses.
type Conn struct {
	AgentID   string
	UserID    int64
	SessionID string

	conn *websocket.Conn

	writeMu sync.Mutex

	mu      sync.Mutex
	waiters map[string]chan Envelope // request_id → channel
}

func newConn(c *websocket.Conn, agentID string, userID int64, sessionID string) *Conn {
	return &Conn{
		AgentID:   agentID,
		UserID:    userID,
		SessionID: sessionID,
		conn:      c,
		waiters:   map[string]chan Envelope{},
	}
}

// Send writes one envelope to the underlying socket, serialized against
// other goroutines that may also be writing (heartbeat, query relay).
func (c *Conn) Send(ctx context.Context, env Envelope) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	return wsjson.Write(ctx, c.conn, env)
}

// Subscribe registers a channel that will receive every Envelope whose
// payload references the given request_id. The caller MUST call the
// returned unsubscribe function when done to release the slot.
func (c *Conn) Subscribe(requestID string) (<-chan Envelope, func()) {
	ch := make(chan Envelope, 16)
	c.mu.Lock()
	c.waiters[requestID] = ch
	c.mu.Unlock()
	return ch, func() {
		c.mu.Lock()
		if existing, ok := c.waiters[requestID]; ok && existing == ch {
			delete(c.waiters, requestID)
			close(ch)
		}
		c.mu.Unlock()
	}
}

// dispatch routes one incoming envelope to its waiter. Returns true if a
// waiter consumed it.
func (c *Conn) dispatch(requestID string, env Envelope) bool {
	c.mu.Lock()
	ch, ok := c.waiters[requestID]
	c.mu.Unlock()
	if !ok {
		return false
	}
	select {
	case ch <- env:
		return true
	default:
		// Slow consumer, drop. AgentExecutor uses a buffered channel and
		// reads continuously, so this only fires on logic bugs.
		return false
	}
}

// Registry holds every currently-connected agent. Lookups are by agent ID
// (database row PK formatted as decimal string).
type Registry struct {
	mu sync.RWMutex
	m  map[string]*Conn
}

func NewRegistry() *Registry {
	return &Registry{m: map[string]*Conn{}}
}

func (r *Registry) Register(c *Conn) {
	r.mu.Lock()
	if existing, ok := r.m[c.AgentID]; ok {
		// One agent_id, one live connection. Kick the old one.
		_ = existing.conn.Close(websocket.StatusGoingAway, "replaced by new connection")
	}
	r.m[c.AgentID] = c
	r.mu.Unlock()
}

func (r *Registry) Unregister(c *Conn) {
	r.mu.Lock()
	if existing, ok := r.m[c.AgentID]; ok && existing == c {
		delete(r.m, c.AgentID)
	}
	r.mu.Unlock()
}

func (r *Registry) Get(agentID string) (*Conn, bool) {
	r.mu.RLock()
	c, ok := r.m[agentID]
	r.mu.RUnlock()
	return c, ok
}

func (r *Registry) AgentIDs() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]string, 0, len(r.m))
	for id := range r.m {
		out = append(out, id)
	}
	return out
}

// ErrAgentOffline is returned by lookups when the named agent isn't
// currently connected.
var ErrAgentOffline = errors.New("agent offline")

func randID(nBytes int) string {
	b := make([]byte, nBytes)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
