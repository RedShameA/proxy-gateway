package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"

	apperrors "proxygateway/internal/application/apperrors"
)

type AuthFunc func(http.ResponseWriter, *http.Request) bool

const (
	StatusKindBadRequest = apperrors.KindBadRequest
	StatusKindNotFound   = apperrors.KindNotFound
	StatusKindConflict   = apperrors.KindConflict
	StatusKindInternal   = apperrors.KindInternal
	StatusKindBadGateway = apperrors.KindBadGateway
)

type StatusError struct {
	Status  int
	Message string
}

func (err StatusError) Error() string {
	return err.Message
}

func StatusErrorFromKind(err error) error {
	var kindErr apperrors.KindError
	if !errors.As(err, &kindErr) {
		return err
	}
	switch kindErr.Kind() {
	case StatusKindBadRequest:
		return StatusError{Status: http.StatusBadRequest, Message: kindErr.Error()}
	case StatusKindNotFound:
		return StatusError{Status: http.StatusNotFound, Message: kindErr.Error()}
	case StatusKindConflict:
		return StatusError{Status: http.StatusConflict, Message: kindErr.Error()}
	case StatusKindBadGateway:
		return StatusError{Status: http.StatusBadGateway, Message: kindErr.Error()}
	default:
		return StatusError{Status: http.StatusInternalServerError, Message: kindErr.Error()}
	}
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

func writeStatusError(w http.ResponseWriter, err error) {
	if statusErr, ok := StatusErrorFromKind(err).(StatusError); ok {
		writeError(w, statusErr.Status, statusErr.Message)
		return
	}
	writeError(w, http.StatusInternalServerError, err.Error())
}

func readJSON(r *http.Request, out any) error {
	defer r.Body.Close()
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	return dec.Decode(out)
}
