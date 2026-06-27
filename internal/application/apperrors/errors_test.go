package apperrors

import (
	"errors"
	"testing"
)

func TestErrorKeepsKindMessageAndCause(t *testing.T) {
	cause := errors.New("storage failed")
	err := New(KindConflict, "duplicate node", cause)

	var kindErr KindError
	if !errors.As(err, &kindErr) {
		t.Fatalf("New() does not implement KindError")
	}
	if kindErr.Kind() != KindConflict {
		t.Fatalf("Kind() = %q, want %q", kindErr.Kind(), KindConflict)
	}
	if err.Error() != "duplicate node" {
		t.Fatalf("Error() = %q, want duplicate node", err.Error())
	}
	if !errors.Is(err, cause) {
		t.Fatalf("errors.Is() did not preserve cause")
	}
}
