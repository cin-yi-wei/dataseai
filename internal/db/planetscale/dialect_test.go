package planetscale

import (
	"testing"

	"github.com/conray/dataseai/internal/db"
)

func TestEngine(t *testing.T) {
	d := PlanetScale{}
	if d.Engine() != db.EnginePlanetScale {
		t.Errorf("got %q", d.Engine())
	}
}
