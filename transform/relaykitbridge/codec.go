package relaykitbridge

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"

	"github.com/QuantumNous/new-api/relaykit/relayconvert/kitutil"
)

// Go initializes a package once before serving requests. Never switch the
// library-global codec per request: DTO custom decoders use it recursively.
func init() {
	kitutil.SetCodec(numberCodec{})
	// Library anomaly logs can include request content. Returned structured
	// diagnostics remain the host's observable error surface.
	quiet := func(string) {}
	kitutil.SetLogging(quiet, quiet)
	kitutil.SetSystemErrorLogging(quiet)
}

type numberCodec struct{}

func (numberCodec) Marshal(v any) ([]byte, error)       { return json.Marshal(v) }
func (numberCodec) Valid(raw []byte) bool               { return json.Valid(raw) }
func (c numberCodec) Unmarshal(raw []byte, v any) error { return c.Decode(bytes.NewReader(raw), v) }
func (numberCodec) Decode(r io.Reader, v any) error {
	d := json.NewDecoder(r)
	d.UseNumber()
	if err := d.Decode(v); err != nil {
		return err
	}
	var extra any
	if d.Decode(&extra) != io.EOF {
		return errors.New("trailing JSON")
	}
	return nil
}
