package db

import (
	"testing"
)

type stubDialect struct {
	unimplementedDialect
}

func (stubDialect) Engine() Engine     { return Engine("stub") }
func (stubDialect) DriverName() string { return "stub" }

func TestRegisterAndGet(t *testing.T) {
	const e Engine = "stubengine"
	Register(e, stubDialect{})
	got, ok := Lookup(e)
	if !ok {
		t.Fatal("Lookup failed for registered engine")
	}
	if got.DriverName() != "stub" {
		t.Fatalf("driver name = %q, want %q", got.DriverName(), "stub")
	}
}

func TestMustGetPanicsForUnknown(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic for unknown engine")
		}
	}()
	MustGet(Engine("absent"))
}
