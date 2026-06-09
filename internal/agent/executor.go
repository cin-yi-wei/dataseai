package agent

import (
	"context"
	"errors"
	"strconv"

	mysqldialect "github.com/conray/dataseai/internal/db/mysql"
)

type AgentConn interface {
	Send(ctx context.Context, env Envelope) error
	Subscribe(requestID string) (<-chan Envelope, func())
}

type AgentExecutor struct {
	Conn    AgentConn
	Target  MySQLTarget
	Dialect string // "mysql" (default) or "postgres"
}

func (e AgentExecutor) Run(ctx context.Context, statement string, opts mysqldialect.RunOpts) (mysqldialect.ExecResult, error) {
	if e.Conn == nil {
		return mysqldialect.ExecResult{Kind: mysqldialect.Classify(statement)}, ErrAgentOffline
	}
	reqID := randID(8)
	ch, unsubscribe := e.Conn.Subscribe(reqID)
	defer unsubscribe()

	target := e.Target
	// For PG, opts.Database carries the schema name, not the actual PG database.
	// Keep the connection target database unchanged; schema is used in SQL directly.
	isPG := e.Dialect == "postgres" || e.Dialect == "postgresql"
	if opts.Database != "" && !isPG {
		target.Database = opts.Database
	}
	if opts.MaxRows <= 0 {
		opts.MaxRows = 10000
	}
	err := e.Conn.Send(ctx, Envelope{
		Type: TypeQueryRequest,
		Payload: QueryRequest{
			RequestID: reqID,
			Target:    target,
			SQL:       statement,
			Dialect:   e.Dialect,
			MaxRows:   opts.MaxRows,
			BatchSize: 500,
		},
	})
	if err != nil {
		return mysqldialect.ExecResult{Kind: mysqldialect.Classify(statement)}, err
	}

	out := mysqldialect.ExecResult{Kind: mysqldialect.Classify(statement)}
	for {
		select {
		case <-ctx.Done():
			return out, ctx.Err()
		case env, ok := <-ch:
			if !ok {
				return out, ErrAgentOffline
			}
			switch env.Type {
			case TypeQueryMeta:
				var msg QueryMeta
				if err := remarshal(env.Payload, &msg); err != nil {
					return out, err
				}
				out.Columns = make([]string, 0, len(msg.Columns))
				for _, col := range msg.Columns {
					out.Columns = append(out.Columns, col.Name)
				}
			case TypeQueryRows:
				var msg QueryRows
				if err := remarshal(env.Payload, &msg); err != nil {
					return out, err
				}
				out.Rows = append(out.Rows, msg.Rows...)
				if len(out.Rows) >= opts.MaxRows {
					out.Truncated = true
				}
			case TypeQueryDone:
				var msg QueryDone
				if err := remarshal(env.Payload, &msg); err != nil {
					return out, err
				}
				out.DurationMs = msg.DurationMs
				out.RowsAffected = int64(msg.RowCount)
				return out, nil
			case TypeQueryError:
				var msg QueryError
				if err := remarshal(env.Payload, &msg); err != nil {
					return out, err
				}
				if msg.Error == "" {
					return out, errors.New("agent query failed")
				}
				return out, errors.New(msg.Error)
			default:
				return out, errors.New("unexpected agent message type: " + env.Type)
			}
		}
	}
}

func AgentIDString(id int64) string {
	return strconv.FormatInt(id, 10)
}
