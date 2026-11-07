package channelplugin

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"slices"
	"strings"

	"github.com/Calcium-Ion/moejs"
	"github.com/deliciousbuding/metapi-go/internal/channelplugin/contract"
	"github.com/deliciousbuding/metapi-go/internal/requestoverride"
)

const MaxDriverResultBytes = 4 << 20

var providerHooks = []string{"request", "response", "event", "decodeResponse", "decodeEvent", "modelsRequest", "modelsResponse", "submit", "query", "content", "taskResult"}

type EndpointTemplate struct {
	Path      string `json:"path"`
	Auth      string `json:"auth"`
	ModelPath bool   `json:"modelPath,omitempty"`
	Profile   string `json:"profile,omitempty"`
}

type CatalogTemplate struct {
	Path            string `json:"path"`
	Auth            string `json:"auth"`
	CursorParameter string `json:"cursorParameter,omitempty"`
}

type ProviderDefaults struct {
	Endpoints map[string]EndpointTemplate `json:"endpoints,omitempty"`
	Catalog   *CatalogTemplate            `json:"catalog,omitempty"`
}

func validTemplatePath(path, auth string) bool {
	if !strings.HasPrefix(path, "/") || strings.HasPrefix(path, "//") || strings.ContainsAny(path, "?#\\\r\n\x00") || len(path) > 8192 || !slices.Contains([]string{"bearer", "x-api-key", "x-goog-api-key", "none"}, auth) {
		return false
	}
	decoded, err := url.PathUnescape(path)
	if err != nil || strings.ContainsAny(decoded, "\\\r\n\x00") {
		return false
	}
	for _, part := range strings.Split(decoded, "/") {
		if part == "." || part == ".." {
			return false
		}
	}
	return true
}

func (m Manifest) IsProvider() bool { return m.APIVersion == 2 && m.Kind == "provider" }

func (m Manifest) validateProviderLifecycle() error {
	if m.Authentication != nil && (m.Authentication.Validate() != nil || len(m.CredentialKinds) != 1 || m.CredentialKinds[0] != "api_key") {
		return ErrManifest
	}
	if m.RequestKinds != nil && (len(m.RequestKinds) == 0 || len(m.RequestKinds) > 2) {
		return ErrManifest
	}
	seenKinds := map[string]bool{}
	for _, kind := range m.RequestKinds {
		if seenKinds[kind] || kind != "json" && kind != "multipart" {
			return ErrManifest
		}
		seenKinds[kind] = true
	}
	for protocol, observation := range m.Observations {
		if !slices.Contains(m.Protocols, protocol) || observation.Validate() != nil {
			return ErrManifest
		}
	}
	if m.Defaults != nil {
		if len(m.Defaults.Endpoints) > contract.MaxProviderProtocols {
			return ErrManifest
		}
		for protocol, endpoint := range m.Defaults.Endpoints {
			if !slices.Contains(m.Protocols, protocol) || !validTemplatePath(endpoint.Path, endpoint.Auth) || len(endpoint.Profile) > 64 {
				return ErrManifest
			}
		}
		if catalog := m.Defaults.Catalog; catalog != nil {
			if !validTemplatePath(catalog.Path, catalog.Auth) || !slices.Contains(m.Hooks, "modelsRequest") || !contract.ValidCursorParameter(catalog.CursorParameter) {
				return ErrManifest
			}
		}
	}
	if len(m.CredentialKinds) == 0 || len(m.CredentialKinds) > 4 {
		return ErrManifest
	}
	seen := map[string]bool{}
	for _, kind := range m.CredentialKinds {
		if seen[kind] || !slices.Contains([]string{"api_key", "oauth", "gcp_service_account", "none"}, kind) {
			return ErrManifest
		}
		seen[kind] = true
	}
	if slices.Contains(m.Hooks, "modelsRequest") != slices.Contains(m.Hooks, "modelsResponse") {
		return ErrManifest
	}
	for _, protocol := range m.Protocols {
		required := []string{"request", "response"}
		switch protocol {
		case "responsesWebsocket":
			required = []string{"request", "event"}
		case "video", "seedanceVideo", "zenmuxVideo":
			required = []string{"submit", "query", "content", "taskResult"}
		}
		for _, hook := range required {
			if !slices.Contains(m.Hooks, hook) {
				return ErrManifest
			}
		}
	}
	return nil
}

// The host supplies the selected identity and opaque task references. Secrets,
// file bytes, request handles, route selection and billing never enter JS.
type DriverInput struct {
	Protocol       string                     `json:"protocol,omitempty"`
	Body           string                     `json:"body"`
	BodyKind       string                     `json:"bodyKind,omitempty"`
	Files          []FileReference            `json:"files,omitempty"`
	Model          string                     `json:"model,omitempty"`
	Stream         bool                       `json:"stream"`
	CredentialKind string                     `json:"credentialKind"`
	TaskID         string                     `json:"taskId,omitempty"`
	State          json.RawMessage            `json:"state,omitempty"`
	StatusCode     int                        `json:"statusCode,omitempty"`
	Headers        http.Header                `json:"headers,omitempty"`
	Options        map[string]json.RawMessage `json:"options"`
}

type FileReference struct {
	Ref      string `json:"ref"`
	Field    string `json:"field"`
	Filename string `json:"filename"`
	MIMEType string `json:"mimeType"`
	Size     int64  `json:"size"`
}

func (m Manifest) AcceptsRequestKind(kind string) bool {
	if len(m.RequestKinds) == 0 {
		return kind == "json"
	}
	return slices.Contains(m.RequestKinds, kind)
}

func SupportsMultipartProtocol(protocol string) bool {
	return slices.Contains([]string{"video", "seedanceVideo", "zenmuxVideo", "imageEdit", "imageVariation", "audioTranscription", "audioTranslation"}, protocol)
}

// Driver calls produce bounded JSON for the operation's owning host parser.
// Descriptor validation, raw usage observation and terminal validation remain
// mandatory at those consumers; a driver call performs no network I/O.
func (p *Program) CallDriver(ctx context.Context, hook string, input DriverInput) (json.RawMessage, error) {
	if !p.manifest.IsProvider() || !slices.Contains(p.manifest.Hooks, hook) || !slices.Contains(p.manifest.CredentialKinds, input.CredentialKind) {
		return nil, ErrManifest
	}
	if hook == "modelsRequest" || hook == "modelsResponse" {
		if input.Protocol != "" || input.Stream {
			return nil, ErrManifest
		}
	} else if !slices.Contains(p.manifest.Protocols, input.Protocol) || input.Stream && !slices.Contains(p.manifest.Hooks, "event") {
		return nil, ErrManifest
	}
	options, err := p.manifest.ResolveOptions(input.Options)
	if hook == "request" || hook == "submit" {
		kind := input.BodyKind
		if kind == "" {
			kind = "json"
		}
		if !p.manifest.AcceptsRequestKind(kind) {
			return nil, ErrRequestKind
		}
		if kind == "json" && len(input.Files) != 0 || kind == "multipart" && !SupportsMultipartProtocol(input.Protocol) {
			return nil, ErrManifest
		}
	} else if input.BodyKind != "" || len(input.Files) != 0 {
		return nil, ErrResult
	}
	if len(input.Files) > 16 {
		return nil, ErrResult
	}
	seenFiles := map[string]bool{}
	for _, file := range input.Files {
		if file.Ref == "" || len(file.Ref) > 128 || seenFiles[file.Ref] || !validPartName(file.Field) || len(file.Filename) > 1024 || strings.ContainsAny(file.Filename+file.MIMEType, "\r\n\x00") || len(file.MIMEType) > 256 || file.Size < 0 {
			return nil, ErrResult
		}
		seenFiles[file.Ref] = true
	}
	if err != nil {
		return nil, err
	}
	input.Options = options
	rawInput, err := json.Marshal(input)
	if err != nil || len(rawInput) > MaxInputBytes {
		return nil, ErrResult
	}
	var result json.RawMessage
	err = p.run(ctx, func(rt *moejs.Runtime) error {
		argument, err := rt.ParseJSON(rawInput)
		if err != nil {
			return ErrResult
		}
		handle, err := p.module.Hook(hook)
		if err != nil {
			return ErrExecution
		}
		value, err := rt.Call(handle, argument)
		if err != nil {
			return ErrExecution
		}
		if !value.IsObject() {
			return ErrResult
		}
		if _, _, promise := value.AsObject().PromiseResult(); promise {
			return ErrResult
		}
		raw, err := rt.AppendJSON(nil, value)
		if err != nil || len(raw) > MaxDriverResultBytes || len(raw) == 0 || raw[0] != '{' {
			return ErrResult
		}
		result = slices.Clone(raw)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

// A descriptor addresses an administrator-selected endpoint, not a new origin.
// The host injects current authentication after this validation and retains its
// own transport, proxy, cancellation, attempt and redirection policies.
type RequestDescriptor struct {
	Method      string            `json:"method"`
	Path        string            `json:"path,omitempty"`
	Body        string            `json:"body,omitempty"`
	ContentType string            `json:"contentType,omitempty"`
	Headers     map[string]string `json:"headers,omitempty"`
	Parts       []MultipartPart   `json:"parts,omitempty"`
}

type MultipartPart struct {
	Field   string  `json:"field"`
	Value   *string `json:"value,omitempty"`
	FileRef string  `json:"fileRef,omitempty"`
}

func validPartName(name string) bool {
	return name != "" && len(name) <= 256 && !strings.ContainsAny(name, "\r\n\x00")
}

func (d RequestDescriptor) ResolveURL(endpoint string) (*url.URL, error) {
	if d.Validate() != nil {
		return nil, ErrResult
	}
	base, err := url.Parse(endpoint)
	if err != nil || base.User != nil || base.Host == "" || base.Fragment != "" || (base.Scheme != "http" && base.Scheme != "https") {
		return nil, ErrResult
	}
	reference, err := d.relativePath()
	if err != nil {
		return nil, err
	}
	resolved := base.ResolveReference(reference)
	if resolved.Scheme != base.Scheme || resolved.Host != base.Host {
		return nil, ErrResult
	}
	return resolved, nil
}

func (d RequestDescriptor) relativePath() (*url.URL, error) {
	reference, err := url.Parse(d.Path)
	if err != nil || reference.IsAbs() || reference.Host != "" || reference.User != nil || reference.Fragment != "" || strings.ContainsAny(d.Path, "\\\r\n\x00") || len(d.Path) > 8192 {
		return nil, ErrResult
	}
	return reference, nil
}

func (d RequestDescriptor) Validate() error {
	if d.Parts != nil {
		if d.Method != "POST" || d.Body != "" || d.ContentType != "" || len(d.Parts) == 0 || len(d.Parts) > 528 {
			return ErrResult
		}
		for _, part := range d.Parts {
			if !validPartName(part.Field) || (part.Value != nil) == (part.FileRef != "") || len(part.FileRef) > 128 || part.Value != nil && len(*part.Value) > MaxInputBytes {
				return ErrResult
			}
		}
		for key := range d.Headers {
			if strings.EqualFold(key, "Content-Type") || strings.EqualFold(key, "Content-Length") {
				return ErrResult
			}
		}
	}
	if !slices.Contains([]string{"GET", "HEAD", "POST", "PUT", "PATCH", "DELETE"}, d.Method) || len(d.Body) > MaxInputBytes || len(d.Headers) > 64 || len(d.ContentType) > 256 || strings.ContainsAny(d.ContentType, "\r\n\x00") {
		return ErrResult
	}
	if (d.Method == "GET" || d.Method == "HEAD") && d.Body != "" {
		return ErrResult
	}
	if _, err := d.relativePath(); err != nil {
		return err
	}
	var headers []requestoverride.Operation
	seen := map[string]bool{}
	for key, value := range d.Headers {
		canonical := strings.ToLower(key)
		if len(value) > 8192 || seen[canonical] {
			return ErrResult
		}
		seen[canonical] = true
		headers = append(headers, requestoverride.Operation{Op: "set", Path: key, Value: value})
	}
	if (requestoverride.Config{Headers: headers}).Validate() != nil {
		return ErrResult
	}
	return nil
}
