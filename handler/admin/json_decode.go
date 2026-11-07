package admin

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"

	"github.com/deliciousbuding/metapi-go/config"
	"github.com/deliciousbuding/metapi-go/internal/requestoverride"
)

var (
	errJSONRequestBodyTooLarge = errors.New("request body exceeds maximum size")
	adminJSONBodyLimitBytes    = int64(config.DefaultRequestBodyLimit)
)

// decodeJSONRequest decodes exactly one JSON value from an admin request body.
// Unknown fields remain allowed for API compatibility; trailing JSON is rejected
// so malformed or concatenated payloads cannot partially mutate admin state.
func decodeJSONRequest(r *http.Request, dst any) error {
	_, err := decodeJSONRequestRaw(r, dst)
	return err
}

func decodeJSONRequestRaw(r *http.Request, dst any) ([]byte, error) {
	raw, err := readAdminJSONBody(r.Body)
	if err != nil {
		return nil, err
	}
	if err := rejectDuplicateJSONKeys(raw); err != nil {
		return nil, err
	}

	dec := json.NewDecoder(bytes.NewReader(raw))
	if err := dec.Decode(dst); err != nil {
		return nil, err
	}
	var extra struct{}
	if err := dec.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return nil, fmt.Errorf("request body must contain a single JSON value")
		}
		return nil, err
	}
	return raw, nil
}

// isJSONBodyTooLarge reports whether a decode failure came from a body limit
// rather than malformed JSON. The global path-aware middleware wraps the body
// in http.MaxBytesReader, whose read error surfaces as *http.MaxBytesError;
// the local adminJSONBodyLimitBytes guard returns errJSONRequestBodyTooLarge.
// Callers use this to answer 413 instead of a misleading 400 (audit M15).
func isJSONBodyTooLarge(err error) bool {
	if errors.Is(err, errJSONRequestBodyTooLarge) {
		return true
	}
	var maxBytesErr *http.MaxBytesError
	return errors.As(err, &maxBytesErr)
}

func readAdminJSONBody(body io.Reader) ([]byte, error) {
	if adminJSONBodyLimitBytes <= 0 {
		return io.ReadAll(body)
	}
	limited := &io.LimitedReader{R: body, N: adminJSONBodyLimitBytes + 1}
	raw, err := io.ReadAll(limited)
	if err != nil {
		return nil, err
	}
	if int64(len(raw)) > adminJSONBodyLimitBytes {
		return nil, errJSONRequestBodyTooLarge
	}
	return raw, nil
}

func rejectDuplicateJSONKeys(raw []byte) error {
	return requestoverride.RejectDuplicateJSONKeys(raw)
}
