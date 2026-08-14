package proxy

// ProxyLogEntry is a single proxy log row.
type ProxyLogEntry struct {
	RouteID            *int64
	ChannelID          *int64
	AccountID          *int64
	DownstreamAPIKeyID *int64
	ModelRequested     string
	ModelActual        *string
	Status             string
	HTTPStatus         int
	IsStream           *bool
	FirstByteLatencyMs *int64
	LatencyMs          int64
	PromptTokens       *int64
	CompletionTokens   *int64
	TotalTokens        *int64
	EstimatedCost      float64
	BillingDetails     any
	ClientFamily       string
	ClientAppID        string
	ClientAppName      string
	ClientConfidence   string
	ErrorMessage       *string
	RetryCount         int
	// RequestID correlates multi-channel retry/failover attempts for one client call.
	// Same value as chi X-Request-Id / middleware.GetReqID for the ingress request.
	RequestID    string
	UpstreamPath *string
	UsageSource  string
}
