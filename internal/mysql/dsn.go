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
}

// BuildDSN constructs a go-sql-driver/mysql DSN.
// Format: user:password@tcp(host:port)/dbname?param=value
func BuildDSN(in DSNInput) string {
	tlsParam := "false"
	switch in.TLS {
	case "required":
		tlsParam = "true"
	case "preferred":
		tlsParam = "preferred"
	}
	user := url.QueryEscape(in.Username)
	pass := url.QueryEscape(in.Password)
	return fmt.Sprintf(
		"%s:%s@tcp(%s:%d)/%s?parseTime=true&tls=%s&charset=utf8mb4",
		user, pass, in.Host, in.Port, in.DefaultDB, tlsParam,
	)
}
