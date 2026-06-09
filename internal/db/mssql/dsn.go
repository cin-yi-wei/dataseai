package mssql

import (
	"fmt"
	"net/url"

	"github.com/conray/dataseai/internal/db"
)

// BuildDSN returns a sqlserver:// URL DSN. When in.Network is non-empty it
// is an SSH-connector reference registered by RegisterSSHDialer — return it
// verbatim so the SSH driver wrapper looks it up.
func (MSSQL) BuildDSN(in db.DSNInput) string {
	if in.Network != "" {
		return in.Network
	}
	return buildBaseDSN(in)
}

func buildBaseDSN(in db.DSNInput) string {
	u := &url.URL{
		Scheme: "sqlserver",
		Host:   fmt.Sprintf("%s:%d", in.Host, in.Port),
	}
	if in.Username != "" {
		u.User = url.UserPassword(in.Username, in.Password)
	}
	q := url.Values{}
	if in.DefaultDB != "" {
		q.Set("database", in.DefaultDB)
	}
	switch in.TLS {
	case "disabled":
		q.Set("encrypt", "disable")
	case "skip-verify":
		q.Set("encrypt", "true")
		q.Set("TrustServerCertificate", "true")
	case "required":
		q.Set("encrypt", "true")
	default:
		// "preferred" or empty: opportunistic
		q.Set("encrypt", "false")
	}
	q.Set("dial timeout", "30")
	u.RawQuery = q.Encode()
	return u.String()
}
