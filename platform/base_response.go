package platform

import (
	"encoding/json"
	"fmt"
	"strings"
)

func getString(m map[string]interface{}, key string) (string, bool) {
	v, ok := m[key]
	if !ok {
		return "", false
	}
	s, ok := v.(string)
	return s, ok
}

func getBool(m map[string]interface{}, key string) (bool, bool) {
	v, ok := m[key]
	if !ok {
		return false, false
	}
	b, ok := v.(bool)
	return b, ok
}

func getFloat(m map[string]interface{}, key string) (float64, bool) {
	v, ok := m[key]
	if !ok {
		return 0, false
	}
	f, ok := v.(float64)
	return f, ok
}

func getIntPtr(m map[string]interface{}, key string) *int {
	v, ok := m[key]
	if !ok {
		return nil
	}
	switch n := v.(type) {
	case float64:
		i := int(n)
		return &i
	case int:
		return &n
	case json.Number:
		i, err := n.Int64()
		if err == nil {
			val := int(i)
			return &val
		}
	}
	return nil
}

func getMap(m map[string]interface{}, key string) (map[string]interface{}, bool) {
	v, ok := m[key]
	if !ok {
		return nil, false
	}
	mm, ok := v.(map[string]interface{})
	return mm, ok
}

func extractResponseMessage(payload map[string]interface{}) string {
	if msg, ok := getString(payload, "message"); ok && strings.TrimSpace(msg) != "" {
		return strings.TrimSpace(msg)
	}
	if errObj, ok := getMap(payload, "error"); ok {
		if msg, ok := getString(errObj, "message"); ok && strings.TrimSpace(msg) != "" {
			return strings.TrimSpace(msg)
		}
	}
	if msg, ok := getString(payload, "msg"); ok && strings.TrimSpace(msg) != "" {
		return strings.TrimSpace(msg)
	}
	return ""
}

func extractResponseMessageFromBytes(body []byte) string {
	var m map[string]interface{}
	if err := json.Unmarshal(body, &m); err != nil {
		return ""
	}
	return extractResponseMessage(m)
}

// parseOneApiStyleBalance parses a OneAPI-style /api/user/self balance payload.
// Accepts either the full response envelope or the extracted "data" map.
// quotaMeansRemaining selects between forks where "quota" is the remaining
// amount (balance=quota, total=quota+used) vs the total (balance=quota-used).
func parseOneApiStyleBalance(resp map[string]interface{}, divisor float64, quotaMeansRemaining bool) BalanceInfo {
	data, ok := getMap(resp, "data")
	if !ok {
		data = resp
	}

	quota, _ := getFloat(data, "quota")
	used, _ := getFloat(data, "used_quota")

	quotaUSD := quota / divisor
	usedUSD := used / divisor
	balanceUSD := quotaUSD - usedUSD
	if quotaMeansRemaining {
		balanceUSD = quotaUSD
		quotaUSD += usedUSD // total = remaining + used
	}

	var todayIncome *float64
	if v, ok := getFloat(data, "today_income"); ok {
		ti := v / divisor
		todayIncome = &ti
	}
	var todayQuotaConsumption *float64
	if v, ok := getFloat(data, "today_quota_consumption"); ok {
		tq := v / divisor
		todayQuotaConsumption = &tq
	}

	return BalanceInfo{
		Balance:               balanceUSD,
		Used:                  usedUSD,
		Quota:                 quotaUSD,
		TodayIncome:           todayIncome,
		TodayQuotaConsumption: todayQuotaConsumption,
	}
}

// checkinResultFromResponse builds a CheckinResult from a checkin endpoint
// response, falling back to successMsg/failureMsg when the message is empty.
func checkinResultFromResponse(resp map[string]interface{}, successMsg, failureMsg string) *CheckinResult {
	success, _ := getBool(resp, "success")
	msg, _ := getString(resp, "message")
	if msg == "" {
		if success {
			msg = successMsg
		} else {
			msg = failureMsg
		}
	}
	result := &CheckinResult{Success: success, Message: msg}
	if success {
		if data, ok := getMap(resp, "data"); ok {
			if reward, ok := data["reward"]; ok {
				result.Reward = fmt.Sprintf("%v", reward)
			}
		}
	}
	return result
}
