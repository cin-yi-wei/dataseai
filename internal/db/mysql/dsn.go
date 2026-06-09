package mysql

import (
	"fmt"
	"time"

	gomysql "github.com/go-sql-driver/mysql"

	"github.com/conray/dataseai/internal/db"
)

// BuildDSN constructs a go-sql-driver/mysql DSN via the driver's own Config
// struct. Building by hand and url.QueryEscape'ing the password is wrong —
// the driver does NOT URL-decode the password section when parsing a DSN,
// so any escaping we do gets sent literally to MySQL. FormatDSN() handles
// the right escape rules per field.
func (MySQL) BuildDSN(in db.DSNInput) string {
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
	cfg.Timeout = 30 * time.Second
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
