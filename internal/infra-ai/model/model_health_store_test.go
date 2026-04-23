package model

import (
	"testing"
	"time"
)

func TestModelHealthStoreOpensAfterFailures(t *testing.T) {
	store := NewModelHealthStore()

	store.markFailure("m1")

	if store.allowCall("m1") {
		t.Fatal("expected call to be blocked while circuit is open")
	}
	if !store.isUnavailable("m1") {
		t.Fatal("expected model to be unavailable")
	}
}

func TestModelHealthStoreHalfOpenAllowsSingleProbe(t *testing.T) {
	store := NewModelHealthStore()
	store.healthByID.Store("m1", &modelHealth{
		state:     Open,
		openUntil: time.Now().Add(-time.Millisecond),
	})

	if !store.allowCall("m1") {
		t.Fatal("expected first call after open window to be allowed")
	}
	if store.allowCall("m1") {
		t.Fatal("expected second half-open probe to be blocked")
	}
}

func TestModelHealthStoreSuccessClosesCircuit(t *testing.T) {
	store := NewModelHealthStore()
	store.healthByID.Store("m1", &modelHealth{
		state:            HalfOpen,
		halfOpenInFlight: true,
	})

	store.markSuccess("m1")

	if !store.allowCall("m1") {
		t.Fatal("expected calls to be allowed after success")
	}
	if store.isUnavailable("m1") {
		t.Fatal("expected model to be available after success")
	}
}
