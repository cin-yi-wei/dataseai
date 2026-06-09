package clickhouse

import (
	"fmt"
	"net/url"

	"github.com/conray/dataseai/internal/db"
)

// BuildDSN returns a clickhouse:// URL DSN for clickhouse-go/v2.
// When in.Network is non-empty it is the local SSH forwarder address.
func (CH) BuildDSN(in db.DSNInput) string {
	addr := fmt.Sprintf("%s:%d", in.Host, in.Port)
	if in.Network != "" {
		addr = in.Network
	}
	u := &url.URL{
		Scheme: "clickhouse",
		Host:   addr,
		Path:   "/" + in.DefaultDB,
	}
	if in.Username != "" || in.Password != "" {
		u.User = url.UserPassword(in.Username, in.Password)
	}
	q := url.Values{}
	q.Set("dial_timeout", "10s")
	q.Set("compress", "lz4")
	switch in.TLS {
	case "required":
		q.Set("secure", "true")
	case "skip-verify":
		q.Set("secure", "true")
		q.Set("skip_verify", "true")
	case "disabled":
		q.Set("secure", "false")
	}
	u.RawQuery = q.Encode()
	return u.String()
}
