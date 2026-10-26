package upstreamadapter

// Choice is the native Chat lifecycle information consumed by the SSE owner.
type Choice struct {
	Index    int
	Finished bool
}
