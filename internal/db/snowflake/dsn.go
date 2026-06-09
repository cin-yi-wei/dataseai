package snowflake

import (
	"fmt"
	"strings"

	"github.com/conray/dataseai/internal/db"
	sf "github.com/snowflakedb/gosnowflake"
)

// BuildDSN constructs a Snowflake DSN using the official gosnowflake DSN builder.
// Host should be the Snowflake account identifier (e.g. "xy12345.us-east-1"
// or "myorg-myaccount"). The .snowflakecomputing.com suffix is stripped if present.
func (Snowflake) BuildDSN(in db.DSNInput) string {
	if in.Network != "" {
		return in.Network
	}
	account := strings.TrimSuffix(in.Host, ".snowflakecomputing.com")

	cfg := &sf.Config{
		Account:  account,
		User:     in.Username,
		Password: in.Password,
		Database: in.DefaultDB,
	}
	if in.Port != 0 && in.Port != 443 {
		cfg.Port = in.Port
	}
	dsn, err := sf.DSN(cfg)
	if err != nil {
		// Fallback: manual assembly
		return fmt.Sprintf("%s:%s@%s/%s", in.Username, in.Password, account, in.DefaultDB)
	}
	return dsn
}
