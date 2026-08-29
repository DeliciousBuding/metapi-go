package proxyhandler

import (
	"os"
	"testing"

	"github.com/deliciousbuding/metapi-go/config"
)

func TestMain(m *testing.M) {
	// Initialize config before tests run (PrepareCtx calls config.Get());
	// the hot-path failure judge / first-byte guard read the runtime
	// snapshot, so publish an empty baseline too.
	config.Set(&config.Config{
		ProxyMaxChannelAttempts: 10,
	})
	config.SetRuntime(&config.RuntimeSettings{})
	// Most proxy surface tests exercise the historical local stub path.
	// Production keeps the stub disabled unless this flag is set explicitly.
	os.Setenv("METAPI_ENABLE_PROXY_STUB", "1")
	os.Exit(m.Run())
}
