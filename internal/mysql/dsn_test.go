package mysql

import (
	"testing"

	gomysql "github.com/go-sql-driver/mysql"
)

// TestBuildDSN round-trips the DSN through the driver's own ParseDSN to make
// sure the fields land back where they came from. We deliberately don't
// compare BuildDSN's exact string output: that's an implementation detail
// of FormatDSN (e.g. param ordering, default collation) that changes
// between driver versions. What matters is that the password — including
// the awkward chars that previously broke url.QueryEscape — survives the
// trip without mutation.
func TestBuildDSN(t *testing.T) {
	cases := []struct {
		name    string
		in      DSNInput
		wantTLS string
	}{
		{"simple", DSNInput{Host: "h", Port: 3306, Username: "u", Password: "p", TLS: "disabled"}, "false"},
		{"awkward pw", DSNInput{Host: "h", Port: 3307, Username: "u", Password: "p:@/", DefaultDB: "mydb", TLS: "required"}, "true"},
		{"preferred", DSNInput{Host: "h", Port: 3306, Username: "u", Password: "p", TLS: "preferred"}, "preferred"},
		{"skip-verify", DSNInput{Host: "h", Port: 3306, Username: "u", Password: "p", TLS: "skip-verify"}, "skip-verify"},
		{"slash in pw (sandbox_ssh repro)", DSNInput{Host: "h", Port: 3306, Username: "fatgame", Password: "vup2u/3yj04", TLS: "disabled"}, "false"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dsn := BuildDSN(c.in)
			cfg, err := gomysql.ParseDSN(dsn)
			if err != nil {
				t.Fatalf("ParseDSN(%q) failed: %v", dsn, err)
			}
			if cfg.User != c.in.Username {
				t.Errorf("user: got %q want %q (dsn=%q)", cfg.User, c.in.Username, dsn)
			}
			if cfg.Passwd != c.in.Password {
				t.Errorf("passwd: got %q want %q (dsn=%q)", cfg.Passwd, c.in.Password, dsn)
			}
			wantAddr := c.in.Host + ":3306"
			if c.in.Port != 0 && c.in.Port != 3306 {
				wantAddr = c.in.Host + ":3307"
			}
			if cfg.Addr != wantAddr {
				t.Errorf("addr: got %q want %q", cfg.Addr, wantAddr)
			}
			if cfg.DBName != c.in.DefaultDB {
				t.Errorf("dbname: got %q want %q", cfg.DBName, c.in.DefaultDB)
			}
			if cfg.TLSConfig != c.wantTLS {
				t.Errorf("tls: got %q want %q", cfg.TLSConfig, c.wantTLS)
			}
			if !cfg.ParseTime {
				t.Errorf("ParseTime should be true")
			}
		})
	}
}
