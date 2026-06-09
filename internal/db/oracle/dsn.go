package oracle

import (
	"strconv"

	"github.com/conray/dataseai/internal/db"
	go_ora "github.com/sijms/go-ora/v2"
)

// BuildDSN returns an oracle:// URL for go-ora/v2.
// Host is the Oracle DB host, DefaultDB is the service name or SID.
func (Oracle) BuildDSN(in db.DSNInput) string {
	host := in.Host
	port := in.Port
	if port == 0 {
		port = 1521
	}
	if in.Network != "" {
		// SSH forwarder: "host:port" string
		h, p := splitHostPort(in.Network, port)
		host = h
		port = p
	}
	options := map[string]string{}
	switch in.TLS {
	case "required":
		options["ssl"] = "true"
	case "skip-verify":
		options["ssl"] = "true"
		options["ssl verify"] = "false"
	}
	return go_ora.BuildUrl(host, port, in.DefaultDB, in.Username, in.Password, options)
}

func splitHostPort(addr string, defaultPort int) (string, int) {
	for i := len(addr) - 1; i >= 0; i-- {
		if addr[i] == ':' {
			p, err := strconv.Atoi(addr[i+1:])
			if err == nil {
				return addr[:i], p
			}
			break
		}
	}
	return addr, defaultPort
}
