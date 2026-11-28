package responses

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// Chat clients decode created into int64. Shift the decimal point using the
// JSON lexeme so integral timestamps above 2^53 stay exact even in exponent form.
func bridgeCreatedAt(value any) (int64, error) {
	invalid := fmt.Errorf("Responses/Chat bridge: created_at must be an int64 timestamp")
	number, ok := value.(json.Number)
	if !ok {
		return 0, invalid
	}
	if n, err := number.Int64(); err == nil {
		return n, nil
	}
	raw := number.String()
	mantissa, exponent, hasExponent := strings.Cut(strings.ToLower(raw), "e")
	unsigned, negative := strings.CutPrefix(mantissa, "-")
	whole, fraction, _ := strings.Cut(unsigned, ".")
	digits := strings.TrimLeft(whole+fraction, "0")
	if digits == "" {
		return 0, nil
	}
	exp := 0
	if hasExponent {
		var err error
		exp, err = strconv.Atoi(exponent)
		if err != nil || exp < -len(digits) || exp > len(fraction)+19 {
			return 0, invalid
		}
	}
	coefficient := strings.TrimRight(digits, "0")
	zeros := exp - len(fraction) + (len(digits) - len(coefficient))
	if zeros < 0 || zeros > 19-len(coefficient) {
		return 0, invalid
	}
	if negative {
		coefficient = "-" + coefficient
	}
	n, err := strconv.ParseInt(coefficient+strings.Repeat("0", zeros), 10, 64)
	if err != nil {
		return 0, invalid
	}
	return n, nil
}

func bridgeUsage(raw any, fromChat bool) (bridgeObject, error) {
	if raw == nil {
		return nil, nil
	}
	usage := bridgeMap(raw)
	if usage == nil {
		return nil, fmt.Errorf("Responses/Chat bridge: invalid usage")
	}
	in, out := "input_tokens", "output_tokens"
	if fromChat {
		in, out = "prompt_tokens", "completion_tokens"
	}
	result := bridgeObject{}
	for _, pair := range [][2]string{{in, "input_tokens"}, {out, "output_tokens"}, {"total_tokens", "total_tokens"}} {
		v, exists := usage[pair[0]]
		if !exists {
			continue
		}
		n, ok := v.(json.Number)
		if !ok {
			return nil, fmt.Errorf("Responses/Chat bridge: usage count must be numeric")
		}
		count, err := n.Int64()
		if err != nil || count < 0 {
			return nil, fmt.Errorf("Responses/Chat bridge: invalid usage count")
		}
		result[pair[1]] = n
	}
	for _, pair := range [][2]string{{in + "_details", "input_tokens_details"}, {out + "_details", "output_tokens_details"}} {
		if v := usage[pair[0]]; v != nil {
			details := bridgeMap(v)
			if details == nil {
				return nil, fmt.Errorf("Responses/Chat bridge: invalid usage details")
			}
			for _, count := range details {
				n, ok := count.(json.Number)
				if !ok {
					return nil, fmt.Errorf("Responses/Chat bridge: invalid usage detail count")
				}
				i, err := n.Int64()
				if err != nil || i < 0 {
					return nil, fmt.Errorf("Responses/Chat bridge: invalid usage detail count")
				}
			}
			result[pair[1]] = v
		}
	}
	if !fromChat {
		for _, pair := range [][2]string{{"input_tokens", "prompt_tokens"}, {"output_tokens", "completion_tokens"}, {"input_tokens_details", "prompt_tokens_details"}, {"output_tokens_details", "completion_tokens_details"}} {
			if v, exists := result[pair[0]]; exists {
				result[pair[1]] = v
				delete(result, pair[0])
			}
		}
	}
	return result, nil
}

func bridgeRejectError(obj bridgeObject) error {
	if obj["error"] != nil {
		return fmt.Errorf("Responses/Chat bridge: upstream returned an error")
	}
	return nil
}
