package apperrors

const (
	KindBadRequest = "bad_request"
	KindNotFound   = "not_found"
	KindConflict   = "conflict"
	KindInternal   = "internal"
	KindBadGateway = "bad_gateway"
)

type KindError interface {
	error
	Kind() string
}

type Error struct {
	kind    string
	message string
	err     error
}

func New(kind, message string, err error) error {
	return Error{kind: kind, message: message, err: err}
}

func (err Error) Error() string {
	if err.message != "" {
		return err.message
	}
	if err.err != nil {
		return err.err.Error()
	}
	return err.kind
}

func (err Error) Unwrap() error {
	return err.err
}

func (err Error) Kind() string {
	return err.kind
}
