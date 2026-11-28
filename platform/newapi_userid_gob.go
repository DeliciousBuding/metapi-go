package platform

func extractGobFieldInts(payload []byte, fieldName string) []int {
	var ids []int
	seen := make(map[int]bool)
	push := func(id int) {
		if id <= 0 || id > 10_000_000 || seen[id] {
			return
		}
		seen[id] = true
		ids = append(ids, id)
	}

	// Build marker: fieldName + 0x03 + "int" + 0x04
	marker := append([]byte(fieldName), 0x03)
	marker = append(marker, []byte("int")...)
	marker = append(marker, 0x04)

	start := 0
	for start < len(payload) {
		pos := indexOf(payload, marker, start)
		if pos < 0 {
			break
		}

		markerEnd := pos + len(marker)
		if markerEnd+1 >= len(payload) {
			start = pos + len(marker)
			continue
		}

		encodedLength := payload[markerEnd]
		delimiter := payload[markerEnd+1]
		if delimiter != 0x00 {
			start = pos + len(marker)
			continue
		}

		byteLength := int(encodedLength) - 1
		if byteLength <= 0 || markerEnd+2+byteLength > len(payload) {
			start = pos + len(marker)
			continue
		}

		valueBytes := payload[markerEnd+2 : markerEnd+2+byteLength]
		if id := decodeGobSignedInt(valueBytes); id > 0 {
			push(id)
		}

		start = pos + len(marker)
	}

	return ids
}

func indexOf(data, sub []byte, start int) int {
	for i := start; i <= len(data)-len(sub); i++ {
		match := true
		for j := 0; j < len(sub); j++ {
			if data[i+j] != sub[j] {
				match = false
				break
			}
		}
		if match {
			return i
		}
	}
	return -1
}

func decodeGobSignedInt(encoded []byte) int {
	if len(encoded) == 0 {
		return 0
	}

	var unsigned uint64
	if encoded[0] < 0x80 {
		unsigned = uint64(encoded[0])
	} else {
		width := 0x100 - int(encoded[0])
		if width <= 0 || len(encoded) != width+1 {
			return 0
		}
		for i := 1; i < len(encoded); i++ {
			unsigned = (unsigned << 8) | uint64(encoded[i])
		}
	}

	// zigzag decode
	var signed int64
	if unsigned&1 == 0 {
		signed = int64(unsigned >> 1)
	} else {
		signed = -int64((unsigned >> 1) + 1)
	}

	if signed <= 0 || signed > 10_000_000 {
		return 0
	}
	return int(signed)
}
