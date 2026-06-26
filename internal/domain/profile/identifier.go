package profile

import "errors"

var (
	ErrIdentifierLength  = errors.New("profile identifier length")
	ErrIdentifierCharset = errors.New("profile identifier charset")
)

func ValidateIdentifier(value string) error {
	if value == "" {
		return nil
	}
	if len(value) < 3 || len(value) > 32 {
		return ErrIdentifierLength
	}
	for _, c := range value {
		if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '-' || c == '_' {
			continue
		}
		return ErrIdentifierCharset
	}
	return nil
}
