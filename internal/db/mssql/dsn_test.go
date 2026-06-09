package mssql

import (
	"strings"
	"testing"

	"github.com/conray/dataseai/internal/db"
)

func TestBuildDSNBasics(t *testing.T) {
	d := MSSQL{}
	got := d.BuildDSN(db.DSNInput{
		Host: "sqlhost", Port: 1433, Username: "sa", Password: "secret", DefaultDB: "mydb",
	})
	for _, want := range []string{"sqlserver://", "sqlhost:1433", "sa", "database=mydb"} {
		if !strings.Contains(got, want) {
			t.Fatalf("DSN missing %q: %s", want, got)
		}
	}
}

func TestBuildDSNTLSDisabled(t *testing.T) {
	d := MSSQL{}
	got := d.BuildDSN(db.DSNInput{Host: "h", Port: 1433, Username: "u", TLS: "disabled"})
	if !strings.Contains(got, "encrypt=disable") {
		t.Fatalf("encrypt=disable missing: %q", got)
	}
}

func TestBuildDSNTLSRequired(t *testing.T) {
	d := MSSQL{}
	got := d.BuildDSN(db.DSNInput{Host: "h", Port: 1433, Username: "u", TLS: "required"})
	if !strings.Contains(got, "encrypt=true") {
		t.Fatalf("encrypt=true missing: %q", got)
	}
}

func TestBuildDSNTLSSkipVerify(t *testing.T) {
	d := MSSQL{}
	got := d.BuildDSN(db.DSNInput{Host: "h", Port: 1433, Username: "u", TLS: "skip-verify"})
	if !strings.Contains(got, "TrustServerCertificate=true") {
		t.Fatalf("TrustServerCertificate missing: %q", got)
	}
}

func TestBuildDSNNetworkPassthrough(t *testing.T) {
	d := MSSQL{}
	ref := "mssql-ssh:42"
	got := d.BuildDSN(db.DSNInput{Network: ref})
	if got != ref {
		t.Fatalf("Network passthrough broken: %q", got)
	}
}
