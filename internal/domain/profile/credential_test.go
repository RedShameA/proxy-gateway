package profile

import (
	"errors"
	"testing"
)

func TestValidateProxyCredential(t *testing.T) {
	if err := ValidateProxyCredential("client laptop", "abc-DEF_123"); err != nil {
		t.Fatalf("ValidateProxyCredential valid = %v", err)
	}
	if err := ValidateProxyCredential("  ", "abc123"); !errors.Is(err, ErrCredentialRemarkRequired) {
		t.Fatalf("blank remark error = %v, want ErrCredentialRemarkRequired", err)
	}
	if err := ValidateProxyCredential("client", "short"); !errors.Is(err, ErrCredentialPasswordLength) {
		t.Fatalf("short password error = %v, want ErrCredentialPasswordLength", err)
	}
	if err := ValidateProxyCredential("client", "123456789012345678901234567890123"); !errors.Is(err, ErrCredentialPasswordLength) {
		t.Fatalf("long password error = %v, want ErrCredentialPasswordLength", err)
	}
	if err := ValidateProxyCredential("client", "abc.def"); !errors.Is(err, ErrCredentialPasswordCharset) {
		t.Fatalf("charset error = %v, want ErrCredentialPasswordCharset", err)
	}
}
