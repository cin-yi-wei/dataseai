package mysql

import (
	"fmt"
	"net/url"
)

type DSNInput struct {
	Host      string
	Port      int
	Username  string
	Password  string
	DefaultDB string
	TLS       string // "disabled" | "preferred" | "required"
	// Network overrides the DSN's network portion (default "tcp"). Used by
	// the SSH tunnel code to inject a custom dialer.
	Network string
}

// BuildDSN constructs a go-sql-driver/mysql DSN.
// Format: user:password@network(host:port)/dbname?param=value
func BuildDSN(in DSNInput) string {
	tlsParam := "false"
	switch in.TLS {
	case "required":
		tlsParam = "true"
	case "preferred":
		tlsParam = "preferred"
	}
	network := in.Network
	if network == "" {
		network = "tcp"
	}
	user := url.QueryEscape(in.Username)
	pass := url.QueryEscape(in.Password)
	return fmt.Sprintf(
		"%s:%s@%s(%s:%d)/%s?parseTime=true&tls=%s&charset=utf8mb4",
		user, pass, network, in.Host, in.Port, in.DefaultDB, tlsParam,
	)
}
