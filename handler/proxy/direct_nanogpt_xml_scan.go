package proxyhandler

import (
	"bytes"
	"fmt"
)

type nanoGPTXMLPart struct {
	start, end int
	node       *nanoGPTXMLNode
	name       string
	err        error
}
type nanoGPTXMLBlock struct {
	offset   int
	boundary *nanoGPTXMLBoundary
	parts    []nanoGPTXMLPart
}

func (p *nanoGPTText) scanBlock(final bool) (consumed int, ready, plain bool, err error) {
	block := p.block
	for {
		if block.boundary == nil {
			for block.offset < len(p.buffer) && (p.buffer[block.offset] == ' ' || p.buffer[block.offset] == '\t' || p.buffer[block.offset] == '\r') {
				block.offset++
			}
			if block.offset == len(p.buffer) {
				return block.offset, final, false, nil
			}
			if p.buffer[block.offset] == '\n' {
				return block.offset, true, false, nil
			}
			if p.buffer[block.offset] != '<' {
				return 0, false, true, nil
			}
			block.boundary = &nanoGPTXMLBoundary{position: block.offset, start: block.offset}
		}
		end, complete := block.boundary.advance(p.buffer)
		if !complete {
			// An unclosed element is text, never a repaired or invented tool.
			if final {
				block.parts = nil
				return len(p.buffer), true, false, nil
			}
			return 0, false, false, nil
		}
		start := block.boundary.start
		node, decodeErr := nanoGPTDecodeXML(p.buffer[start:end])
		if decodeErr != nil {
			node = nil // Unknown/malformed documents stay opaque and unchanged.
		}
		block.parts = append(block.parts, nanoGPTXMLPart{start: start, end: end, node: node, name: nanoGPTOpeningName(p.buffer[start:end]), err: decodeErr})
		if len(block.parts) > directNanoGPTMaxItems {
			return 0, false, false, fmt.Errorf("NanoGPT XML block exceeds element limit")
		}
		block.offset, block.boundary = end, nil
	}
}

// The lexical boundary scan is incremental and quote/CDATA/comment aware. XML
// is decoded exactly once when a complete root has arrived, avoiding repeated
// decoding of a growing tool argument on every SSE content fragment.
type nanoGPTXMLBoundary struct {
	start, position, tagStart, depth int
	inTag                            bool
	quote                            byte
	special                          string
}

func (b *nanoGPTXMLBoundary) advance(data []byte) (int, bool) {
	for b.position < len(data) {
		if !b.inTag {
			if data[b.position] != '<' {
				b.position++
				continue
			}
			b.tagStart = b.position
			remaining := data[b.position:]
			pending := false
			for _, prefix := range []string{"<!--", "<![CDATA[", "<?"} {
				if len(remaining) < len(prefix) && bytes.HasPrefix([]byte(prefix), remaining) {
					pending = true
				}
			}
			if pending {
				return 0, false
			}
			b.inTag = true
			switch {
			case bytes.HasPrefix(remaining, []byte("<!--")):
				b.special = "-->"
				b.position += 4
			case bytes.HasPrefix(remaining, []byte("<![CDATA[")):
				b.special = "]]>"
				b.position += 9
			case bytes.HasPrefix(remaining, []byte("<?")):
				b.special = "?>"
				b.position += 2
			default:
				b.position++
			}
		}
		if b.special != "" {
			index := bytes.Index(data[b.position:], []byte(b.special))
			if index < 0 {
				b.position = max(b.position, len(data)-len(b.special)+1)
				return 0, false
			}
			b.position += index + len(b.special)
			b.inTag = false
			b.special = ""
			if b.depth == 0 {
				return b.position, true
			}
			continue
		}
		for b.position < len(data) {
			c := data[b.position]
			b.position++
			if b.quote != 0 {
				if c == b.quote {
					b.quote = 0
				}
				continue
			}
			if c == '\'' || c == '"' {
				b.quote = c
				continue
			}
			if c != '>' {
				continue
			}
			tag := bytes.TrimSpace(data[b.tagStart:b.position])
			if bytes.HasPrefix(tag, []byte("</")) {
				b.depth--
			} else if !bytes.HasSuffix(tag, []byte("/>")) && !bytes.HasPrefix(tag, []byte("<!")) {
				b.depth++
			}
			b.inTag = false
			if b.depth <= 0 {
				return b.position, true
			}
			break
		}
	}
	return 0, false
}

func nanoGPTOpeningName(raw []byte) string {
	if len(raw) == 0 || raw[0] != '<' {
		return ""
	}
	end := 1
	for end < len(raw) && !bytes.ContainsRune([]byte(" \t\r\n/>"), rune(raw[end])) {
		end++
	}
	return string(raw[1:end])
}
