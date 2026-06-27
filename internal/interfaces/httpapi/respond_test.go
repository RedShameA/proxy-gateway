package httpapi

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

type fakeKindError struct {
	kind string
}

func (err fakeKindError) Error() string {
	return "mapped error"
}

func (err fakeKindError) Kind() string {
	return err.kind
}

func TestStatusErrorFromKindMapsKnownKinds(t *testing.T) {
	tests := []struct {
		kind string
		want int
	}{
		{StatusKindBadRequest, http.StatusBadRequest},
		{StatusKindNotFound, http.StatusNotFound},
		{StatusKindConflict, http.StatusConflict},
		{StatusKindBadGateway, http.StatusBadGateway},
		{StatusKindInternal, http.StatusInternalServerError},
		{"unexpected", http.StatusInternalServerError},
	}

	for _, tt := range tests {
		t.Run(tt.kind, func(t *testing.T) {
			err := StatusErrorFromKind(fakeKindError{kind: tt.kind})
			statusErr, ok := err.(StatusError)
			if !ok {
				t.Fatalf("StatusErrorFromKind type = %T", err)
			}
			if statusErr.Status != tt.want || statusErr.Message != "mapped error" {
				t.Fatalf("status error = %#v, want status %d", statusErr, tt.want)
			}
		})
	}
}

func TestStatusErrorFromKindReturnsOriginalErrorWithoutStatusKind(t *testing.T) {
	original := errors.New("plain")

	err := StatusErrorFromKind(original)
	if err != original {
		t.Fatalf("StatusErrorFromKind = %v, want original", err)
	}
}

func TestWriteStatusErrorMapsKindError(t *testing.T) {
	rec := httptest.NewRecorder()

	writeStatusError(rec, fakeKindError{kind: StatusKindNotFound})

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}
