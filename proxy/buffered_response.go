package proxy

import (
	"errors"
	"fmt"
	"io"

	"github.com/deliciousbuding/metapi-go/config"
)

// DefaultMaxBufferedResponseBodyBytes is retained as the local fallback used
// only when the global config singleton has not been loaded yet (e.g. tests
// that bypass TestMain). Production callers go through Load(), which resolves
// PROXY_MAX_BUFFERED_RESPONSE_BYTES once at startup and stores the value on
// config.Config.ProxyMaxBufferedResponseBytes.
const DefaultMaxBufferedResponseBodyBytes int64 = 20 << 20

var ErrBufferedResponseBodyTooLarge = errors.New("upstream response body exceeded buffered limit")

// MaxBufferedResponseBodyBytes returns the configured max non-streaming
// upstream response size in bytes from the startup-loaded config singleton.
// Falls back to DefaultProxyMaxBufferedResponseBytes when config is not yet
// loaded (e.g. tests that bypass TestMain) or when the operator left the
// value at zero/negative (Load already clamps, but GetSafe guards anyway).
// Reading a struct field per request is materially cheaper than re-parsing
// os.Getenv + strconv.ParseInt on every proxied request — this function is
// on the hot path called from upstream.go, upstream_stream.go, executor.go.
func MaxBufferedResponseBodyBytes() int64 {
	if cfg := config.GetSafe(); cfg != nil && cfg.ProxyMaxBufferedResponseBytes > 0 {
		return int64(cfg.ProxyMaxBufferedResponseBytes)
	}
	return int64(config.DefaultProxyMaxBufferedResponseBytes)
}

func ReadBufferedResponseBody(r io.Reader) ([]byte, error) {
	limit := MaxBufferedResponseBodyBytes()
	data, err := io.ReadAll(io.LimitReader(r, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("%w: limit %d bytes", ErrBufferedResponseBodyTooLarge, limit)
	}
	return data, nil
}
