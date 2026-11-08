// Package channelplugin executes compiled channel wire hooks. It deliberately
// has no database, credential, routing, filesystem or network capability.
package channelplugin

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math"
	"regexp"
	"slices"
	"sync"
	"time"

	"github.com/Calcium-Ion/moejs"
	"github.com/deliciousbuding/metapi-go/internal/channelplugin/contract"
	"github.com/deliciousbuding/metapi-go/internal/requestoverride"
)

const (
	MaxSourceBytes = 256 << 10
	MaxInputBytes  = 4 << 20
	MaxResultBytes = 256 << 10
	CallTimeout    = 250 * time.Millisecond
	Concurrency    = 4
	MemoryLimit    = 16 << 20
)

var (
	ErrManifest    = errors.New("channel_plugin_manifest_invalid")
	ErrRequestKind = errors.New("channel_plugin_request_kind_unsupported")
	ErrCompile     = errors.New("channel_plugin_compile_failed")
	ErrExecution   = errors.New("channel_plugin_execution_failed")
	ErrResult      = errors.New("channel_plugin_result_invalid")
	versionPattern = regexp.MustCompile(`^(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(?:-[A-Za-z0-9.-]+)?$`)
)

type Option struct {
	Key     string          `json:"key"`
	Label   string          `json:"label"`
	Type    string          `json:"type"`
	Default json.RawMessage `json:"default"`
	Choices []string        `json:"choices,omitempty"`
	Min     *float64        `json:"min,omitempty"`
	Max     *float64        `json:"max,omitempty"`
}

type Manifest struct {
	APIVersion      int                             `json:"apiVersion"`
	Kind            string                          `json:"kind,omitempty"`
	Key             string                          `json:"key"`
	Name            string                          `json:"name"`
	Version         string                          `json:"version"`
	Protocols       []string                        `json:"protocols"`
	Hooks           []string                        `json:"hooks"`
	Options         []Option                        `json:"options,omitempty"`
	CredentialKinds []string                        `json:"credentialKinds,omitempty"`
	RequestKinds    []string                        `json:"requestKinds,omitempty"`
	Defaults        *ProviderDefaults               `json:"defaults,omitempty"`
	Observations    map[string]ProtocolObservations `json:"observations,omitempty"`
	Authentication  *contract.Authentication        `json:"authentication,omitempty"`
	TaskCapture     *string                         `json:"taskCapture,omitempty"`
	ImageTasks      map[string]ImageTaskTemplate    `json:"imageTasks,omitempty"`
}

func (m Manifest) Validate() error {
	maxProtocols, validProtocol, hooks := contract.MaxProtocols, contract.ValidProtocol, []string{"request", "response", "event"}
	if m.IsProvider() {
		if m.validateProviderLifecycle() != nil {
			return ErrManifest
		}
		maxProtocols, validProtocol, hooks = contract.MaxProviderProtocols, contract.ValidProviderProtocol, providerHooks
	} else if m.APIVersion != 1 || m.Kind != "" || len(m.CredentialKinds) != 0 || m.RequestKinds != nil || m.Defaults != nil || m.Observations != nil || m.Authentication != nil || m.TaskCapture != nil || m.ImageTasks != nil {
		return ErrManifest
	}
	if !contract.ValidKey(m.Key) || len(m.Name) == 0 || len(m.Name) > 128 || !versionPattern.MatchString(m.Version) || len(m.Options) > 32 || len(m.Protocols) == 0 || len(m.Protocols) > maxProtocols || len(m.Hooks) == 0 || len(m.Hooks) > len(hooks) {
		return ErrManifest
	}
	seen := map[string]bool{}
	for _, protocol := range m.Protocols {
		if seen[protocol] || !validProtocol(protocol) {
			return ErrManifest
		}
		seen[protocol] = true
	}
	seen = map[string]bool{}
	for _, hook := range m.Hooks {
		if seen[hook] || !slices.Contains(hooks, hook) {
			return ErrManifest
		}
		seen[hook] = true
	}
	seen = map[string]bool{}
	for _, option := range m.Options {
		if option.Type != "number" && option.Type != "integer" && (option.Min != nil || option.Max != nil) {
			return ErrManifest
		}
		if !contract.ValidKey(option.Key) || seen[option.Key] || len(option.Label) == 0 || len(option.Label) > 128 || len(option.Choices) > 64 {
			return ErrManifest
		}
		seen[option.Key] = true
		if !option.valid(option.Default) {
			return ErrManifest
		}
	}
	return nil
}

func (o Option) valid(raw json.RawMessage) bool {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || len(raw) > 4096 {
		return false
	}
	switch o.Type {
	case "boolean":
		var value bool
		return string(raw) != "null" && len(o.Choices) == 0 && json.Unmarshal(raw, &value) == nil
	case "string", "select":
		var value string
		if string(raw) == "null" || json.Unmarshal(raw, &value) != nil {
			return false
		}
		if o.Type == "string" {
			return len(o.Choices) == 0
		}
		return len(o.Choices) > 0 && slices.Contains(o.Choices, value)
	case "number", "integer":
		if o.Min == nil || o.Max == nil || !finiteOptionNumber(*o.Min) || !finiteOptionNumber(*o.Max) || *o.Min > *o.Max || len(o.Choices) > 0 {
			return false
		}
		var number json.Number
		if string(raw) == "null" || raw[0] == '"' || json.Unmarshal(raw, &number) != nil {
			return false
		}
		value, err := number.Float64()
		if err != nil || !finiteOptionNumber(value) || value < *o.Min || value > *o.Max {
			return false
		}
		return o.Type == "number" || math.Trunc(value) == value
	default:
		return false
	}
}

func finiteOptionNumber(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0) && math.Abs(value) <= 1e9
}

// ResolveOptions rejects unknown keys and invalid types; defaults belong to
// the installed manifest, not a second set maintained by a form or importer.
func (m Manifest) ResolveOptions(values map[string]json.RawMessage) (map[string]json.RawMessage, error) {
	resolved := make(map[string]json.RawMessage, len(m.Options))
	for _, option := range m.Options {
		value, ok := values[option.Key]
		if !ok {
			value = option.Default
		}
		if !option.valid(value) {
			return nil, ErrManifest
		}
		resolved[option.Key] = slices.Clone(value)
	}
	for key := range values {
		if _, ok := resolved[key]; !ok {
			return nil, ErrManifest
		}
	}
	return resolved, nil
}

// Input contains only per-attempt wire data and manifest-validated options.
// JSON travels as text, so passing an untouched field through a hook cannot
// round a 64-bit integer, reorder members or strip native extensions.
type Input struct {
	Protocol string                     `json:"protocol"`
	Body     string                     `json:"body"`
	Model    string                     `json:"model"`
	Stream   bool                       `json:"stream"`
	Options  map[string]json.RawMessage `json:"options"`
}

// Hooks calculate ordered operations rather than replacing routing, transport,
// credential or budget decisions. Values are JSON text, not JS Numbers.
type Result struct {
	Body    []requestoverride.Operation `json:"body,omitempty"`
	Headers []requestoverride.Operation `json:"headers,omitempty"`
}

type Program struct {
	manifest    Manifest
	module      *moejs.Module
	admission   chan struct{}
	fingerprint [32]byte
}

func (p *Program) Fingerprint() string { return hex.EncodeToString(p.fingerprint[:]) }

func (p *Program) Manifest() Manifest {
	raw, _ := json.Marshal(p.manifest)
	var out Manifest
	_ = json.Unmarshal(raw, &out)
	return out
}

// Compile evaluates metadata in the same bounded environment as request hooks.
// Raw engine exceptions are never returned: plugins could echo private input
// or credentials accidentally embedded in source into a management/log error.
func Compile(ctx context.Context, source string) (*Program, error) {
	if len(source) == 0 || len(source) > MaxSourceBytes {
		return nil, ErrCompile
	}
	module, err := moejs.Compile("channel-plugin.js", source)
	if err != nil || len(module.Requests()) != 0 {
		return nil, ErrCompile
	}
	p := &Program{module: module, admission: make(chan struct{}, Concurrency)}
	err = p.run(ctx, func(rt *moejs.Runtime) error {
		value, ok := rt.Export("meta")
		if !ok {
			return ErrManifest
		}
		raw, err := rt.AppendJSON(nil, value)
		if err != nil || len(raw) > MaxResultBytes || requestoverride.Decode(raw, &p.manifest) != nil || p.manifest.Validate() != nil {
			return ErrManifest
		}
		for _, name := range p.manifest.Hooks {
			hook, err := p.module.Hook(name)
			if err != nil {
				return ErrManifest
			}
			ok, err := rt.Has(hook)
			if err != nil || !ok {
				return ErrManifest
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	metadata, _ := json.Marshal(p.manifest)
	p.fingerprint = sha256.Sum256([]byte(source + "\x00" + string(metadata)))
	return p, nil
}

func (p *Program) Call(ctx context.Context, hook string, input Input) (Result, error) {
	var result Result
	if p.manifest.IsProvider() || !slices.Contains(p.manifest.Protocols, input.Protocol) {
		return result, ErrManifest
	}
	if !slices.Contains(p.manifest.Hooks, hook) {
		return result, nil
	}
	options, err := p.manifest.ResolveOptions(input.Options)
	if err != nil {
		return result, err
	}
	input.Options = options
	rawInput, err := json.Marshal(input)
	if err != nil || len(rawInput) > MaxInputBytes {
		return result, ErrResult
	}
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
		if value.IsObject() {
			if _, _, promise := value.AsObject().PromiseResult(); promise {
				return ErrResult
			}
		}
		raw, err := rt.AppendJSON(nil, value)
		if err != nil || len(raw) > MaxResultBytes || string(raw) == "null" || requestoverride.Decode(raw, &result) != nil {
			return ErrResult
		}
		if (requestoverride.Config{Body: result.Body, Headers: result.Headers}).Validate() != nil {
			return ErrResult
		}
		if hook != "request" && len(result.Headers) > 0 {
			return ErrResult
		}
		return nil
	})
	if err != nil {
		return Result{}, err
	}
	return result, nil
}

// Fresh runtimes isolate mutable module state between tenants and calls. Only
// immutable bytecode is shared. Admission covers initialization and result
// serialization too, including malicious getters, jobs and toJSON callbacks.
func (p *Program) run(parent context.Context, call func(*moejs.Runtime) error) error {
	ctx, cancel := context.WithTimeout(parent, CallTimeout)
	defer cancel()
	select {
	case p.admission <- struct{}{}:
	case <-ctx.Done():
		return ctx.Err()
	}
	defer func() { <-p.admission }()
	if err := ctx.Err(); err != nil {
		return err
	}
	rt := moejs.NewRuntime(moejs.Options{DisableDynamicCode: true, MemoryLimit: MemoryLimit, TimeZone: time.UTC})
	var watcher sync.WaitGroup
	watcher.Add(1)
	done := make(chan struct{})
	go func() {
		defer watcher.Done()
		select {
		case <-ctx.Done():
			rt.Interrupt(ctx.Err())
		case <-done:
		}
	}()
	defer func() { close(done); watcher.Wait(); rt.ReleaseCallData() }()
	if err := rt.Load(p.module); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return ErrExecution
	}
	err := call(rt)
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return err
}
