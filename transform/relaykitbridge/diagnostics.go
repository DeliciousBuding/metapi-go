package relaykitbridge

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/QuantumNous/new-api/relaykit/types"
)

// Diagnostic is the public, content-free classification of a conversion loss.
type Diagnostic struct {
	Phase      string `json:"phase"`
	Code       string `json:"code"`
	Severity   string `json:"severity"`
	FieldClass string `json:"fieldClass"`
}

type conversionError struct {
	phase       string
	diagnostics []Diagnostic
}

func (e *conversionError) Error() string {
	if len(e.diagnostics) == 0 {
		return e.phase + ": " + ErrConversion.Error()
	}
	d := e.diagnostics[0]
	return fmt.Sprintf("%s: %s (%s, %s, %s)", d.Phase, ErrConversion, d.Code, d.Severity, d.FieldClass)
}
func (e *conversionError) Unwrap() error                    { return ErrConversion }
func (e *conversionError) DiagnosticsSummary() []Diagnostic { return slices.Clone(e.diagnostics) }

func (s *Session) checkDiagnostics(phase string, diagnostics []types.ConversionDiagnostic) error {
	if len(diagnostics) == 0 {
		return nil
	}
	return s.conversionError(phase, nil, diagnostics)
}
func (s *Session) conversionError(phase string, err error, diagnostics []types.ConversionDiagnostic) error {
	var loss *types.ConversionLossError
	if errors.As(err, &loss) {
		diagnostics = loss.Diagnostics
	}
	var summary []Diagnostic
	for _, d := range diagnostics {
		field := d.Path
		if i := strings.IndexAny(field, "[."); i >= 0 {
			field = field[:i]
		}
		switch field {
		case "tools", "tool_choice", "input", "output", "messages", "content", "candidates", "contents":
		default:
			field = "other"
		}
		severity := string(d.Severity)
		switch d.Severity {
		case types.ConversionDiagnosticError, types.ConversionDiagnosticWarning:
		default:
			severity = "unknown"
		}
		safe := Diagnostic{phase, diagnosticCode(d.Code), severity, field}
		if !slices.Contains(summary, safe) {
			summary = append(summary, safe)
		}
	}
	s.diagnostics = summary
	return &conversionError{phase, summary}
}

// RelayKit v0.3.8 diagnostic codes are library constants. Future or dynamically
// supplied strings collapse to a single value until explicitly reviewed.
func diagnosticCode(code string) string {
	switch code {
	case "additional_tools_flattened", "allowed_function_subset_unrepresentable",
		"continuation_state_lost", "custom_tool_as_function", "custom_tool_name_conflict",
		"duplicate_hosted_web_search", "enterprise_search_semantics_unrepresentable",
		"forced_tool_choice_downgraded", "gemini_schema_keyword_dropped",
		"grounding_citation_invalid", "grounding_offset_invalid", "grounding_part_ambiguous",
		"grounding_part_invalid", "grounding_source_invalid", "grounding_text_mismatch",
		"hosted_tool_caller_unrepresentable", "hosted_tool_error_missing",
		"hosted_tool_event_omitted", "hosted_tool_event_unrepresentable",
		"hosted_tool_history_approximated", "hosted_tool_history_unsupported",
		"hosted_tool_id_missing", "hosted_tool_input_invalid", "hosted_tool_metadata_reduced",
		"hosted_tool_order_unrepresentable", "hosted_tool_result_approximated",
		"hosted_tool_result_orphaned", "hosted_tool_status_unrepresentable",
		"hosted_tool_unrepresentable", "implicit_search_choice", "invalid_native_tool_choice",
		"legacy_search_semantics_unrepresentable", "mcp_approval_state_unrepresentable",
		"mcp_arguments_missing", "mcp_arguments_unrepresentable", "mcp_identity_missing",
		"mcp_result_ambiguous", "mcp_result_missing", "mcp_result_unrepresentable",
		"mcp_text_result_normalized", "namespaced_tool_dropped", "tool_choice_without_tools",
		"tool_namespace_flattened", "unsupported_blocked_domains", "unsupported_domain_filter",
		"unsupported_function_strict", "unsupported_hosted_tool",
		"unsupported_native_search_config", "unsupported_opaque_tool",
		"unsupported_parallel_tool_control", "unsupported_search_constraints",
		"unsupported_search_context_size", "unsupported_search_controls",
		"unsupported_search_limit", "unsupported_search_tuning", "unsupported_tool_choice",
		"unverified_search_mapping", "unverified_tool_mapping",
		"web_search_action_unrepresentable", "web_search_call_unrepresentable",
		"web_search_continuation_unrepresentable", "web_search_response_unrepresentable",
		"web_search_result_omitted":
		return code
	}
	return "unknown"
}
