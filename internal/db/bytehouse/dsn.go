package bytehouse

import (
	"fmt"
	"net/url"

	"github.com/conray/dataseai/internal/db"
)

// BuildDSN returns a tcp:// DSN for the bytehouse driver.
// When in.Network is non-empty it is a local SSH forward address
// (host:port) stored by RegisterSSHDialer — build the DSN pointing there.
func (BH) BuildDSN(in db.DSNInput) string {
	host := in.Host
	port := in.Port
	if in.Network != "" {
		// in.Network is "host:port" of the local SSH forwarder
		return buildDSNWithAddr(in, in.Network)
	}
	return buildBaseDSN(host, port, in)
}

func buildBaseDSN(host string, port int, in db.DSNInput) string {
	addr := fmt.Sprintf("%s:%d", host, port)
	return buildDSNWithAddr(in, addr)
}

func buildDSNWithAddr(in db.DSNInput, addr string) string {
	u := &url.URL{
		Scheme: "tcp",
		Host:   addr,
	}
	q := url.Values{}
	if in.Username != "" {
		q.Set("user", in.Username)
	}
	if in.Password != "" {
		q.Set("password", in.Password)
	}
	if in.DefaultDB != "" {
		q.Set("database", in.DefaultDB)
	}
	switch in.TLS {
	case "required", "skip-verify":
		q.Set("secure", "true")
		if in.TLS == "skip-verify" {
			q.Set("skip_verify", "true")
		}
	case "disabled":
		q.Set("secure", "false")
	}
	q.Set("conn_timeout", "30")
	u.RawQuery = q.Encode()
	return u.String()
}
