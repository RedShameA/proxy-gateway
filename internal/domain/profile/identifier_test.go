package profile

import (
	"errors"
	"testing"
)

func TestValidateIdentifier(t *testing.T) {
	for _, value := range []string{"", "abc", "A_B-123", "client-01"} {
		if err := ValidateIdentifier(value); err != nil {
			t.Fatalf("ValidateIdentifier(%q) = %v", value, err)
		}
	}
	if err := ValidateIdentifier("ab"); !errors.Is(err, ErrIdentifierLength) {
		t.Fatalf("short identifier error = %v, want ErrIdentifierLength", err)
	}
	if err := ValidateIdentifier("123456789012345678901234567890123"); !errors.Is(err, ErrIdentifierLength) {
		t.Fatalf("long identifier error = %v, want ErrIdentifierLength", err)
	}
	if err := ValidateIdentifier("client.name"); !errors.Is(err, ErrIdentifierCharset) {
		t.Fatalf("charset error = %v, want ErrIdentifierCharset", err)
	}
}
