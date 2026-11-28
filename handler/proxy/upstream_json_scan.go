package proxyhandler

func isJSONSpace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r'
}

// bytesEqualString compares b to s without allocating.
func bytesEqualString(b []byte, s string) bool {
	if len(b) != len(s) {
		return false
	}
	for i := 0; i < len(s); i++ {
		if b[i] != s[i] {
			return false
		}
	}
	return true
}

// skipJSONString advances *i past the JSON string starting at body[*i]=='"'.
func skipJSONString(body []byte, i *int) bool {
	j := *i + 1
	for j < len(body) {
		switch body[j] {
		case '\\':
			j += 2
		case '"':
			*i = j + 1
			return true
		default:
			j++
		}
	}
	return false
}

func skipJSONLiteral(body []byte, i *int, lit string) bool {
	end := *i + len(lit)
	if end > len(body) {
		return false
	}
	for k := 0; k < len(lit); k++ {
		if body[*i+k] != lit[k] {
			return false
		}
	}
	*i = end
	return true
}

func skipJSONNumber(body []byte, i *int) bool {
	j := *i
	if j < len(body) && body[j] == '-' {
		j++
	}
	digits := 0
	for j < len(body) && body[j] >= '0' && body[j] <= '9' {
		j++
		digits++
	}
	if digits == 0 {
		return false
	}
	if j < len(body) && body[j] == '.' {
		j++
		frac := 0
		for j < len(body) && body[j] >= '0' && body[j] <= '9' {
			j++
			frac++
		}
		if frac == 0 {
			return false
		}
	}
	if j < len(body) && (body[j] == 'e' || body[j] == 'E') {
		j++
		if j < len(body) && (body[j] == '+' || body[j] == '-') {
			j++
		}
		exp := 0
		for j < len(body) && body[j] >= '0' && body[j] <= '9' {
			j++
			exp++
		}
		if exp == 0 {
			return false
		}
	}
	*i = j
	return true
}

// skipJSONContainer advances *i past one balanced {...} or [...] container
// starting at body[*i], ignoring delimiters inside strings.
func skipJSONContainer(body []byte, i *int) bool {
	depth := 0
	j := *i
	for j < len(body) {
		switch body[j] {
		case '"':
			if !skipJSONString(body, &j) {
				return false
			}
			continue
		case '{', '[':
			depth++
		case '}', ']':
			depth--
			if depth == 0 {
				*i = j + 1
				return true
			}
		}
		j++
	}
	return false
}

// skipJSONValue advances *i past one JSON value starting at body[*i].
func skipJSONValue(body []byte, i *int) bool {
	if *i >= len(body) {
		return false
	}
	switch body[*i] {
	case '"':
		return skipJSONString(body, i)
	case '{', '[':
		return skipJSONContainer(body, i)
	case 't':
		return skipJSONLiteral(body, i, "true")
	case 'f':
		return skipJSONLiteral(body, i, "false")
	case 'n':
		return skipJSONLiteral(body, i, "null")
	case '-', '0', '1', '2', '3', '4', '5', '6', '7', '8', '9':
		return skipJSONNumber(body, i)
	}
	return false
}
