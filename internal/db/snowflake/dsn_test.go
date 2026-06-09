package snowflake

import (
	"strings"
	"testing"

	"github.com/conray/dataseai/internal/db"
)

func TestBuildDSN(t *testing.T) {
	s := Snowflake{}

	// Network override bypasses DSN building
	if got := s.BuildDSN(db.DSNInput{Network: "override", Host: "ignored"}); got != "override" {
		t.Errorf("got %q", got)
	}

	// Standard account identifier
	dsn := s.BuildDSN(db.DSNInput{
		Host:      "xy12345.us-east-1",
		Username:  "user",
		Password:  "pass",
		DefaultDB: "mydb",
	})
	if !strings.Contains(dsn, "xy12345.us-east-1") && !strings.Contains(dsn, "user") {
		t.Errorf("DSN missing expected fragments: %q", dsn)
	}

	// Suffix stripping: account identifier must not be doubled in DSN
	dsn2 := s.BuildDSN(db.DSNInput{
		Host:     "xy12345.us-east-1.snowflakecomputing.com",
		Username: "u",
		Password: "p",
	})
	if strings.Contains(dsn2, ".snowflakecomputing.com.snowflakecomputing.com") {
		t.Errorf("DSN doubled suffix: %q", dsn2)
	}
	if !strings.Contains(dsn2, "xy12345.us-east-1") {
		t.Errorf("DSN missing account: %q", dsn2)
	}
}
