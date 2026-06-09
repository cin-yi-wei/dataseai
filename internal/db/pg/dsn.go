package pg

import (
	"fmt"
	"strings"

	"github.com/conray/dataseai/internal/db"
)

// BuildDSN constructs a pgx key=value DSN. pgx accepts both URL and key=value
// formats; we use key=value because it round-trips arbitrary passwords without
// percent-encoding subtleties.
//
// When in.Network is non-empty it is a pre-registered pgx DSN reference
// returned by RegisterSSHDialer. Return it verbatim so pgx looks up the
// registered config instead of trying to parse host/port/etc again.
func (PG) BuildDSN(in db.DSNInput) string {
	if in.Network != "" {
		return in.Network
	}
	return PG{}.buildBaseDSN(in)
}

// buildBaseDSN renders the actual key=value DSN from DSNInput, ignoring the
// Network passthrough. RegisterSSHDialer uses this to feed pgx.ParseConfig
// when constructing an SSH-tunneled ConnConfig.
func (PG) buildBaseDSN(in db.DSNInput) string {
	var b strings.Builder
	add := func(key, val string) {
		if val == "" {
			return
		}
		if b.Len() > 0 {
			b.WriteByte(' ')
		}
		needsQuote := strings.ContainsAny(val, " '\\")
		if needsQuote {
			fmt.Fprintf(&b, `%s='%s'`, key, strings.NewReplacer(`\`, `\\`, `'`, `\'`).Replace(val))
		} else {
			fmt.Fprintf(&b, "%s=%s", key, val)
		}
	}
	add("host", in.Host)
	if in.Port != 0 {
		add("port", fmt.Sprintf("%d", in.Port))
	}
	add("user", in.Username)
	add("password", in.Password)
	add("dbname", in.DefaultDB)
	switch in.TLS {
	case "disabled":
		add("sslmode", "disable")
	case "preferred":
		add("sslmode", "prefer")
	case "required":
		add("sslmode", "require")
	case "skip-verify":
		add("sslmode", "require")
	default:
		add("sslmode", "prefer")
	}
	add("connect_timeout", "30")
	return b.String()
}
