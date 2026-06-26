package profile

import (
	"errors"
	"strings"
)

var (
	ErrCredentialRemarkRequired  = errors.New("proxy credential remark required")
	ErrCredentialPasswordLength  = errors.New("proxy credential password length")
	ErrCredentialPasswordCharset = errors.New("proxy credential password charset")
)

func ValidateProxyCredential(remark, password string) error {
	if strings.TrimSpace(remark) == "" {
		return ErrCredentialRemarkRequired
	}
	if len(password) < 6 || len(password) > 32 {
		return ErrCredentialPasswordLength
	}
	for _, c := range password {
		if (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '-' || c == '_' {
			continue
		}
		return ErrCredentialPasswordCharset
	}
	return nil
}
