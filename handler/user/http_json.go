package user

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"reflect"

	"github.com/deliciousbuding/metapi-go/auth"
	"github.com/deliciousbuding/metapi-go/store"
)

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func fail(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}
func resultError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, auth.ErrUserCredentials), errors.Is(err, auth.ErrGatewayUserAccessDenied):
		fail(w, 401, "Invalid user credentials")
	case errors.Is(err, auth.ErrUserPassword), errors.Is(err, store.ErrGatewayUserInvalid):
		fail(w, 400, "Invalid input; password must contain 12 to 72 bytes")
	case errors.Is(err, sql.ErrNoRows):
		fail(w, 404, "Not found")
	default:
		fail(w, 503, "User service unavailable")
	}
}

// Exact field spelling and a bounded single object prevent metadata/identity
// smuggling through Go JSON's case-insensitive matching or trailing documents.
func decode(r *http.Request, dst any) error {
	raw, err := io.ReadAll(io.LimitReader(r.Body, 16385))
	if err != nil || len(raw) > 16384 {
		return errors.New("invalid body")
	}
	document := json.NewDecoder(bytes.NewReader(raw))
	first, err := document.Token()
	if err != nil || first != json.Delim('{') {
		return errors.New("invalid object")
	}
	fields := map[string]json.RawMessage{}
	for document.More() {
		token, err := document.Token()
		if err != nil {
			return err
		}
		key, ok := token.(string)
		if !ok {
			return errors.New("invalid field")
		}
		if _, exists := fields[key]; exists {
			return errors.New("duplicate field")
		}
		var value json.RawMessage
		if err := document.Decode(&value); err != nil {
			return err
		}
		fields[key] = value
	}
	last, err := document.Token()
	if err != nil || last != json.Delim('}') {
		return errors.New("invalid object")
	}
	if _, err := document.Token(); !errors.Is(err, io.EOF) {
		return errors.New("trailing document")
	}
	typ := reflect.TypeOf(dst).Elem()
	for key, value := range fields {
		known := false
		for i := range typ.NumField() {
			if typ.Field(i).Tag.Get("json") == key {
				known = true
				break
			}
		}
		if !known || bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
			return errors.New("unknown or null field")
		}
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	return d.Decode(dst)
}
