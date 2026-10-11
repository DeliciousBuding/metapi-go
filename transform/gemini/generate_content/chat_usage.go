package generate_content

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"
)

func tokenCount(value any) (int64, error) {
	if value == nil {
		return 0, nil
	}
	var number json.Number
	switch v := value.(type) {
	case json.Number:
		number = v
	case int64:
		number = json.Number(strconv.FormatInt(v, 10))
	case int:
		number = json.Number(strconv.Itoa(v))
	case float64:
		if math.IsNaN(v) || math.IsInf(v, 0) || v != math.Trunc(v) || v > 1<<53 {
			return 0, fmt.Errorf("invalid token count")
		}
		number = json.Number(strconv.FormatFloat(v, 'f', 0, 64))
	default:
		return 0, fmt.Errorf("invalid token count")
	}
	n, err := number.Int64()
	if err != nil || n < 0 {
		return 0, fmt.Errorf("invalid token count")
	}
	return n, nil
}

func tokenSum(a, b int64) (int64, error) {
	if b > math.MaxInt64-a {
		return 0, fmt.Errorf("token count overflow")
	}
	return a + b, nil
}

func tokenCounts(values ...any) ([]int64, error) {
	counts := make([]int64, len(values))
	for i, value := range values {
		var err error
		counts[i], err = tokenCount(value)
		if err != nil {
			return nil, err
		}
	}
	return counts, nil
}

func geminiUsage(meta map[string]any) (map[string]any, error) {
	c, err := tokenCounts(meta["promptTokenCount"], meta["candidatesTokenCount"], meta["thoughtsTokenCount"], meta["cachedContentTokenCount"], meta["totalTokenCount"])
	if err != nil {
		return nil, err
	}
	completion, err := tokenSum(c[1], c[2])
	if err != nil {
		return nil, err
	}
	total := c[4]
	if meta["totalTokenCount"] == nil {
		total, err = tokenSum(c[0], completion)
		if err != nil {
			return nil, err
		}
	}
	return map[string]any{"prompt_tokens": c[0], "completion_tokens": completion, "total_tokens": total, "prompt_tokens_details": map[string]any{"cached_tokens": c[3]}, "completion_tokens_details": map[string]any{"reasoning_tokens": c[2]}}, nil
}

func chatUsage(usage map[string]any) (map[string]any, error) {
	promptDetails, _ := usage["prompt_tokens_details"].(map[string]any)
	completionDetails, _ := usage["completion_tokens_details"].(map[string]any)
	c, err := tokenCounts(usage["prompt_tokens"], usage["completion_tokens"], completionDetails["reasoning_tokens"], promptDetails["cached_tokens"], usage["total_tokens"])
	if err != nil {
		return nil, err
	}
	total := c[4]
	if usage["total_tokens"] == nil {
		total, err = tokenSum(c[0], c[1])
		if err != nil {
			return nil, err
		}
	}
	return map[string]any{"promptTokenCount": c[0], "candidatesTokenCount": max(c[1]-c[2], 0), "thoughtsTokenCount": c[2], "totalTokenCount": total, "cachedContentTokenCount": c[3]}, nil
}
