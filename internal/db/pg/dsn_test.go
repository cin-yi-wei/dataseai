package pg

import (
	"strings"
	"testing"

	"github.com/conray/dataseai/internal/db"
)

func TestBuildDSNBasics(t *testing.T) {
	d := PG{}
	got := d.BuildDSN(db.DSNInput{
		Host: "h", Port: 5432, Username: "u", Password: "p", DefaultDB: "appdb",
	})
	for _, want := range []string{"host=h", "port=5432", "user=u", "password=p", "dbname=appdb"} {
		if !strings.Contains(got, want) {
			t.Fatalf("DSN missing %q: %s", want, got)
		}
	}
}

func TestBuildDSNTLSRequired(t *testing.T) {
	d := PG{}
	got := d.BuildDSN(db.DSNInput{
		Host: "h", Port: 5432, Username: "u", Password: "p", TLS: "required",
	})
	if !strings.Contains(got, "sslmode=require") {
		t.Fatalf("sslmode flag missing in %q", got)
	}
}

func TestBuildDSNTLSPreferred(t *testing.T) {
	d := PG{}
	got := d.BuildDSN(db.DSNInput{Host: "h", Port: 5432, Username: "u", TLS: "preferred"})
	if !strings.Contains(got, "sslmode=prefer") {
		t.Fatalf("sslmode missing: %q", got)
	}
}

func TestBuildDSNTLSDisabled(t *testing.T) {
	d := PG{}
	got := d.BuildDSN(db.DSNInput{Host: "h", Port: 5432, Username: "u", TLS: "disabled"})
	if !strings.Contains(got, "sslmode=disable") {
		t.Fatalf("sslmode missing: %q", got)
	}
}

func TestBuildDSNNetworkPassthrough(t *testing.T) {
	d := PG{}
	got := d.BuildDSN(db.DSNInput{Network: "registeredConnConfig:abc123"})
	if got != "registeredConnConfig:abc123" {
		t.Fatalf("Network passthrough broken: %q", got)
	}
}

func TestBuildDSNAwkwardPassword(t *testing.T) {
	d := PG{}
	got := d.BuildDSN(db.DSNInput{
		Host: "h", Port: 5432, Username: "u", Password: "p:@/", DefaultDB: "x",
	})
	if !strings.Contains(got, "password=") {
		t.Fatalf("password absent: %s", got)
	}
}
