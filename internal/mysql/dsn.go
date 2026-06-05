package mysql

import (
	"fmt"
	"time"

	gomysql "github.com/go-sql-driver/mysql"
)

type DSNInput struct {
	Host      string
	Port      int
	Username  string
	Password  string
	DefaultDB string
	TLS       string // "disabled" | "preferred" | "required" | "skip-verify"
	// Network overrides the DSN's network portion (default "tcp"). Used by
	// the SSH tunnel code to inject a custom dialer.
	Network string
}

// BuildDSN constructs a go-sql-driver/mysql DSN via the driver's own Config
// struct. Building by hand and url.QueryEscape'ing the password is wrong —
// the driver does NOT URL-decode the password section when parsing a DSN,
// so any escaping we do gets sent literally to MySQL. FormatDSN() handles
// the right escape rules per field.
func BuildDSN(in DSNInput) string {
	cfg := gomysql.NewConfig()
	cfg.User = in.Username
	cfg.Passwd = in.Password
	if in.Network != "" {
		cfg.Net = in.Network
	} else {
		cfg.Net = "tcp"
	}
	cfg.Addr = fmt.Sprintf("%s:%d", in.Host, in.Port)
	cfg.DBName = in.DefaultDB
	cfg.ParseTime = true
	cfg.Collation = "utf8mb4_general_ci"
	// Connect-timeout is fine over SSH; read/write deadlines aren't supported
	// by golang.org/x/crypto/ssh's tcpChan, so we leave them at zero.
	cfg.Timeout = 30 * time.Second
	// go-sql-driver/mysql tls values:
	//   false / "" — no TLS
	//   "true"     — TLS required + verify cert
	//   "preferred"— TLS if available, else plain
	//   "skip-verify" — TLS required but skip cert+hostname checks
	switch in.TLS {
	case "required":
		cfg.TLSConfig = "true"
	case "preferred":
		cfg.TLSConfig = "preferred"
	case "skip-verify":
		cfg.TLSConfig = "skip-verify"
	default:
		cfg.TLSConfig = "false"
	}
	return cfg.FormatDSN()
}
