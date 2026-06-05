package chat

import "context"

// Proposal carries everything the orchestrator wants the WS layer to render.
type Proposal struct {
	ID             string
	Database       string
	Table          string
	Operation      string // INSERT|UPDATE|DELETE|TRUNCATE|DDL
	SQL            string
	ExplainSummary string // JSON; empty if EXPLAIN didn't run
}

// Decision is what the WS layer returns after the user clicks Execute or Cancel.
type Decision struct {
	Accept       bool
	RowsAffected *int64 // populated by the gateway if it executed; usually nil and execute happens server-side
	Error        string // populated on failure
}

// ProposalGateway is the bridge between the chat orchestrator and the
// WebSocket layer. The orchestrator calls Propose and BLOCKS until the
// WS layer returns a Decision (user click or session close).
//
// The gateway is also responsible for emitting the matching write_executed /
// write_failed / write_cancelled WS events so the UI sees status transitions.
type ProposalGateway interface {
	Propose(ctx context.Context, p Proposal) (Decision, error)
}
