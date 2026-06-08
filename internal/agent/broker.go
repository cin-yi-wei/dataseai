package agent

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/coder/websocket"
	"github.com/coder/websocket/wsjson"
	"github.com/conray/dataseai/internal/store"
)

const (
	heartbeatSeconds = 30
	readDeadline     = 90 * time.Second
)

// Handler returns the http.HandlerFunc mounted at /agent. It owns the
// connector lifecycle: WS upgrade → hello/auth → register in registry →
// read loop dispatching responses to AgentExecutor waiters → unregister.
func Handler(reg *Registry, st *store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
			OriginPatterns: []string{"*"},
		})
		if err != nil {
			return
		}
		defer conn.CloseNow()
		// Default 32 KiB read limit truncates streamed query row batches.
		// Raise to 64 MiB so large rows fit.
		conn.SetReadLimit(64 << 20)

		ctx, cancel := context.WithCancel(r.Context())
		defer cancel()

		// Initial hello must arrive within 10s.
		helloCtx, helloCancel := context.WithTimeout(ctx, 10*time.Second)
		var first Envelope
		err = wsjson.Read(helloCtx, conn, &first)
		helloCancel()
		if err != nil {
			log.Printf("agent: read hello: %v", err)
			return
		}
		if first.Type != TypeHello {
			rejectClose(ctx, conn, "expected hello as first message")
			return
		}
		var hello Hello
		if err := remarshal(first.Payload, &hello); err != nil {
			rejectClose(ctx, conn, "malformed hello")
			return
		}
		if hello.Token == "" {
			rejectClose(ctx, conn, "missing token")
			return
		}

		dbAgent, err := st.GetAgentByTokenHash(store.HashAgentToken(hello.Token))
		if err != nil {
			log.Printf("agent: bad token (sha256=%s...): %v",
				store.HashAgentToken(hello.Token)[:8], err)
			rejectClose(ctx, conn, "invalid token")
			return
		}

		_ = st.UpdateAgentLastSeen(dbAgent.ID, clientIP(r), hello.OS+"/"+hello.Arch, hello.AgentVersion)

		agentID := strconv.FormatInt(dbAgent.ID, 10)
		sessionID := randID(12)
		ac := newConn(conn, agentID, dbAgent.UserID, sessionID)
		reg.Register(ac)
		defer reg.Unregister(ac)

		log.Printf("agent: connected id=%s user=%d host=%s os=%s/%s version=%s",
			agentID, dbAgent.UserID, hello.Hostname, hello.OS, hello.Arch, hello.AgentVersion)

		if err := ac.Send(ctx, Envelope{
			Type: TypeHelloAck,
			Payload: HelloAck{
				AgentID:          agentID,
				SessionID:        sessionID,
				HeartbeatSeconds: heartbeatSeconds,
			},
		}); err != nil {
			return
		}

		go pingLoop(ctx, ac)

		for {
			var msg Envelope
			readCtx, readCancel := context.WithTimeout(ctx, readDeadline)
			err := wsjson.Read(readCtx, conn, &msg)
			readCancel()
			if err != nil {
				if !isExpectedClose(err) {
					log.Printf("agent %s: read: %v", agentID, err)
				}
				return
			}

			switch msg.Type {
			case TypePong:
				// healthy; nothing to do
			case TypePing:
				if err := ac.Send(ctx, Envelope{
					Type:    TypePong,
					Payload: Pong{Ts: time.Now().UnixMilli()},
				}); err != nil {
					return
				}
			case TypeQueryMeta, TypeQueryRows, TypeQueryDone, TypeQueryError:
				requestID := extractRequestID(msg)
				if requestID == "" {
					log.Printf("agent %s: %s missing request_id", agentID, msg.Type)
					continue
				}
				if !ac.dispatch(requestID, msg) {
					log.Printf("agent %s: %s for unknown request_id=%s",
						agentID, msg.Type, requestID)
				}
				// Update last-seen on any inbound traffic; cheap signal of liveness.
				_ = st.UpdateAgentLastSeen(dbAgent.ID, "", "", "")
			default:
				log.Printf("agent %s: unhandled type %q", agentID, msg.Type)
			}
		}
	}
}

func pingLoop(ctx context.Context, ac *Conn) {
	t := time.NewTicker(heartbeatSeconds * time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if err := ac.Send(ctx, Envelope{
				Type:    TypePing,
				Payload: Ping{Ts: time.Now().UnixMilli()},
			}); err != nil {
				return
			}
		}
	}
}

func rejectClose(ctx context.Context, c *websocket.Conn, reason string) {
	_ = wsjson.Write(ctx, c, Envelope{
		Type:    TypeHelloFail,
		Payload: HelloFail{Reason: reason},
	})
}

func remarshal(payload any, dest any) error {
	b, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, dest)
}

func extractRequestID(msg Envelope) string {
	// All query_* payloads carry request_id as a top-level field; pull it
	// out generically without committing to a concrete type.
	m, ok := msg.Payload.(map[string]any)
	if !ok {
		return ""
	}
	s, _ := m["request_id"].(string)
	return s
}

func clientIP(r *http.Request) string {
	if xf := r.Header.Get("X-Forwarded-For"); xf != "" {
		if i := strings.IndexByte(xf, ','); i > 0 {
			return strings.TrimSpace(xf[:i])
		}
		return strings.TrimSpace(xf)
	}
	addr := r.RemoteAddr
	if i := strings.LastIndexByte(addr, ':'); i > 0 {
		return addr[:i]
	}
	return addr
}

func isExpectedClose(err error) bool {
	var ce websocket.CloseError
	if errors.As(err, &ce) {
		return true
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	return false
}
