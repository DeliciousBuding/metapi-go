package observability

import "testing"

func TestDBConnErrorCounter(t *testing.T) {
	ResetDBConnErrorsForTest()
	if got := DBConnErrorsTotal(); got != 0 {
		t.Fatalf("expected 0 after reset, got %d", got)
	}
	RecordDBConnError()
	RecordDBConnError()
	RecordDBConnError()
	if got := DBConnErrorsTotal(); got != 3 {
		t.Fatalf("expected 3 after three records, got %d", got)
	}
	ResetDBConnErrorsForTest()
	if got := DBConnErrorsTotal(); got != 0 {
		t.Fatalf("expected 0 after reset, got %d", got)
	}
}
