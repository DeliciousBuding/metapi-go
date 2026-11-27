package relaykitbridge

import (
	"bytes"
	"encoding/json"
	"io"

	"github.com/QuantumNous/new-api/relaykit/relayconvert/kitutil"
	"github.com/deliciousbuding/metapi-go/transform/shared"
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
	return shared.DecodeJSON(r, v)
}
