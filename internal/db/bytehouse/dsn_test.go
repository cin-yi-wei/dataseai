package bytehouse

import (
	"strings"
	"testing"

	"github.com/conray/dataseai/internal/db"
)

func TestBuildDSNBasics(t *testing.T) {
	d := BH{}
	got := d.BuildDSN(db.DSNInput{
		Host: "bh-host", Port: 9000, Username: "default", Password: "apikey", DefaultDB: "mydb",
	})
	for _, want := range []string{"tcp://", "bh-host:9000", "user=default", "database=mydb"} {
		if !strings.Contains(got, want) {
			t.Fatalf("DSN missing %q: %s", want, got)
		}
	}
}

func TestBuildDSNTLSRequired(t *testing.T) {
	d := BH{}
	got := d.BuildDSN(db.DSNInput{Host: "h", Port: 9000, TLS: "required"})
	if !strings.Contains(got, "secure=true") {
		t.Fatalf("secure=true missing: %q", got)
	}
}

func TestBuildDSNTLSSkipVerify(t *testing.T) {
	d := BH{}
	got := d.BuildDSN(db.DSNInput{Host: "h", Port: 9000, TLS: "skip-verify"})
	if !strings.Contains(got, "skip_verify=true") {
		t.Fatalf("skip_verify=true missing: %q", got)
	}
}

func TestBuildDSNNetworkPassthrough(t *testing.T) {
	d := BH{}
	localAddr := "127.0.0.1:54321"
	in := db.DSNInput{
		Host: "remote-host", Port: 9000, Username: "u", Password: "p",
		Network: localAddr,
	}
	got := d.BuildDSN(in)
	if !strings.Contains(got, "127.0.0.1:54321") {
		t.Fatalf("local SSH addr not in DSN: %q", got)
	}
}
