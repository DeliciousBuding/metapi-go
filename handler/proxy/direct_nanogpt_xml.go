package proxyhandler

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"strings"
)

const directNanoGPTMaxItems = 128
const directNanoGPTXMLLimit = 100_000

type nanoGPTXMLCall struct{ name, arguments string }
type nanoGPTXMLNode struct {
	name     string
	attrs    map[string]string
	text     strings.Builder
	children []*nanoGPTXMLNode
}
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

// Only standalone XML blocks can be calls. Inline examples, fenced/indented
// code and Markdown block quotes remain text. One entire unknown XML root is
// opaque, so a declared tool nested inside an ordinary document is not invoked.
type nanoGPTText struct {
	tools       *directNanoGPTTools
	buffer      []byte
	plainLine   bool
	fence       byte
	fenceWidth  int
	inlineWidth int
	tickRun     int
	block       *nanoGPTXMLBlock
}

func newNanoGPTText(tools *directNanoGPTTools) *nanoGPTText { return &nanoGPTText{tools: tools} }

func (p *nanoGPTText) feed(text string, final bool) (string, []nanoGPTXMLCall, error) {
	if p.tools == nil || len(p.tools.names) == 0 {
		return text, nil, nil
	}
	p.buffer = append(p.buffer, text...)
	var output strings.Builder
	var calls []nanoGPTXMLCall
	for len(p.buffer) > 0 {
		if p.plainLine {
			end := bytes.IndexByte(p.buffer, '\n')
			if end < 0 {
				p.observeInline(p.buffer)
				output.Write(p.buffer)
				p.buffer = p.buffer[:0]
				break
			}
			p.observeInline(p.buffer[:end+1])
			output.Write(p.buffer[:end+1])
			p.buffer = p.buffer[end+1:]
			p.plainLine = false
			continue
		}
		if p.inlineWidth > 0 || p.tickRun > 0 {
			p.plainLine = true
			continue
		}
		if p.block != nil {
			if len(p.buffer) > directNanoGPTXMLLimit {
				return "", nil, fmt.Errorf("NanoGPT pending XML exceeds byte limit")
			}
			consumed, ready, plain, err := p.scanBlock(final)
			if err != nil {
				return "", nil, err
			}
			if plain {
				p.block = nil
				p.plainLine = true
				continue
			}
			if !ready {
				break
			}
			previous := 0
			for _, part := range p.block.parts {
				output.Write(p.buffer[previous:part.start])
				if part.err != nil && (p.tools.names[part.name] || part.name == "use_tool") {
					return "", nil, fmt.Errorf("invalid NanoGPT tool XML: %w", part.err)
				}
				call, ok, err := nanoGPTXMLTool(part.node, p.tools)
				if err != nil {
					return "", nil, err
				}
				if ok {
					calls = append(calls, call)
				} else {
					output.Write(p.buffer[part.start:part.end])
				}
				previous = part.end
			}
			output.Write(p.buffer[previous:consumed])
			p.buffer = p.buffer[consumed:]
			p.block = nil
			continue
		}
		indent := 0
		for indent < len(p.buffer) && p.buffer[indent] == ' ' {
			indent++
		}
		if indent >= 4 || indent < len(p.buffer) && p.buffer[indent] == '\t' {
			p.plainLine = true
			continue
		}
		if indent == len(p.buffer) && !final {
			break
		}
		if indent == len(p.buffer) {
			output.Write(p.buffer)
			p.buffer = p.buffer[:0]
			break
		}
		first := p.buffer[indent]
		if first == '`' || first == '~' {
			width := 0
			for indent+width < len(p.buffer) && p.buffer[indent+width] == first {
				width++
			}
			if indent+width == len(p.buffer) && !final {
				break
			}
			if width >= 3 && (p.fence == 0 || p.fence == first && width >= p.fenceWidth) {
				end := bytes.IndexByte(p.buffer, '\n')
				if end < 0 && !final {
					break
				}
				if end < 0 {
					end = len(p.buffer)
				}
				rest := bytes.TrimSpace(p.buffer[indent+width : end])
				if p.fence == 0 {
					p.fence, p.fenceWidth = first, width
				} else if len(rest) == 0 {
					p.fence, p.fenceWidth = 0, 0
				}
				if end < len(p.buffer) {
					end++
				}
				output.Write(p.buffer[:end])
				p.buffer = p.buffer[end:]
				continue
			}
		}
		if p.fence == 0 && first == '<' {
			p.block = &nanoGPTXMLBlock{offset: indent}
			continue
		}
		p.plainLine = true
	}
	if len(p.buffer) > directNanoGPTXMLLimit {
		return "", nil, fmt.Errorf("NanoGPT pending markup exceeds byte limit")
	}
	return output.String(), calls, nil
}

func (p *nanoGPTText) observeInline(text []byte) {
	if p.fence != 0 {
		return
	}
	for _, c := range text {
		if c == '`' {
			p.tickRun++
			continue
		}
		if p.tickRun > 0 {
			if p.inlineWidth == 0 {
				p.inlineWidth = p.tickRun
			} else if p.inlineWidth == p.tickRun {
				p.inlineWidth = 0
			}
			p.tickRun = 0
		}
	}
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

func nanoGPTDecodeXML(raw []byte) (*nanoGPTXMLNode, error) {
	decoder := xml.NewDecoder(bytes.NewReader(raw))
	var root *nanoGPTXMLNode
	var stack []*nanoGPTXMLNode
	count := 0
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		switch token := token.(type) {
		case xml.StartElement:
			count++
			if count > directNanoGPTMaxItems || len(stack) > directNanoGPTMaxItems {
				return nil, fmt.Errorf("XML exceeds element limit")
			}
			if token.Name.Space != "" {
				return nil, fmt.Errorf("XML namespaces are not tool names")
			}
			node := &nanoGPTXMLNode{name: token.Name.Local, attrs: make(map[string]string)}
			for _, attr := range token.Attr {
				if attr.Name.Space != "" {
					return nil, fmt.Errorf("XML namespaces are not tool arguments")
				}
				if _, exists := node.attrs[attr.Name.Local]; exists {
					return nil, fmt.Errorf("duplicate XML attribute")
				}
				node.attrs[attr.Name.Local] = attr.Value
			}
			if len(stack) == 0 {
				if root != nil {
					return nil, fmt.Errorf("multiple XML roots")
				}
				root = node
			} else {
				parent := stack[len(stack)-1]
				parent.children = append(parent.children, node)
			}
			stack = append(stack, node)
		case xml.EndElement:
			if len(stack) == 0 {
				return nil, fmt.Errorf("XML closing element has no root")
			}
			stack = stack[:len(stack)-1]
		case xml.CharData:
			if len(stack) > 0 {
				stack[len(stack)-1].text.Write(token)
			} else if len(bytes.TrimSpace(token)) > 0 {
				return nil, fmt.Errorf("text outside XML root")
			}
		case xml.Comment:
			return nil, fmt.Errorf("comments are not tool arguments")
		case xml.Directive, xml.ProcInst:
			return nil, fmt.Errorf("XML directives are not tool calls")
		}
	}
	if root == nil || len(stack) != 0 {
		return nil, fmt.Errorf("incomplete XML root")
	}
	return root, nil
}

func nanoGPTXMLTool(node *nanoGPTXMLNode, tools *directNanoGPTTools) (nanoGPTXMLCall, bool, error) {
	if node == nil || tools == nil {
		return nanoGPTXMLCall{}, false, nil
	}
	name := node.name
	if name == "use_tool" {
		name = node.attrs["name"]
	}
	if !tools.names[name] {
		return nanoGPTXMLCall{}, false, nil
	}
	args := make(map[string]json.RawMessage)
	for key, value := range node.attrs {
		if node.name == "use_tool" && key == "name" {
			continue
		}
		args[key], _ = json.Marshal(value)
	}
	if len(node.children) > 0 {
		if strings.TrimSpace(node.text.String()) != "" {
			return nanoGPTXMLCall{}, false, fmt.Errorf("mixed XML tool text and arguments")
		}
		for _, child := range node.children {
			key := child.name
			if (key == "arg" || key == "parameter") && len(child.attrs) == 1 && child.attrs["name"] != "" {
				key = child.attrs["name"]
			} else if len(child.attrs) > 0 {
				return nanoGPTXMLCall{}, false, fmt.Errorf("ambiguous XML parameter attributes")
			}
			if _, exists := args[key]; exists {
				return nanoGPTXMLCall{}, false, fmt.Errorf("duplicate XML tool argument")
			}
			if len(child.children) != 0 {
				return nanoGPTXMLCall{}, false, fmt.Errorf("nested XML parameter requires an explicit JSON argument")
			}
			args[key], _ = json.Marshal(child.text.String())
		}
	} else if inner := node.text.String(); strings.TrimSpace(inner) != "" {
		trimmed := bytes.TrimSpace([]byte(inner))
		if json.Valid(trimmed) {
			var object map[string]json.RawMessage
			if json.Unmarshal(trimmed, &object) == nil && object != nil {
				// Preserve nested JSON verbatim, but reject duplicate outer keys
				// before merging attributes; map decoding alone would erase them.
				decoder := json.NewDecoder(bytes.NewReader(trimmed))
				_, _ = decoder.Token()
				keys := make(map[string]bool)
				for decoder.More() {
					key, err := decoder.Token()
					if err != nil {
						return nanoGPTXMLCall{}, false, err
					}
					name := key.(string)
					if keys[name] {
						return nanoGPTXMLCall{}, false, fmt.Errorf("duplicate JSON tool argument")
					}
					keys[name] = true
					var value json.RawMessage
					if err := decoder.Decode(&value); err != nil {
						return nanoGPTXMLCall{}, false, err
					}
				}
				for key, value := range object {
					if previous, exists := args[key]; exists && !bytes.Equal(previous, value) {
						return nanoGPTXMLCall{}, false, fmt.Errorf("conflicting XML and JSON tool arguments")
					}
					args[key] = value
				}
			} else {
				if _, exists := args["content"]; exists {
					return nanoGPTXMLCall{}, false, fmt.Errorf("conflicting XML content argument")
				}
				args["content"] = trimmed
			}
		} else {
			key := "content"
			if _, exists := args[key]; exists {
				key = "arg"
			}
			if _, exists := args[key]; exists {
				return nanoGPTXMLCall{}, false, fmt.Errorf("conflicting XML text argument")
			}
			args[key], _ = json.Marshal(inner)
		}
	}
	encoded, err := json.Marshal(args)
	if err != nil {
		return nanoGPTXMLCall{}, false, err
	}
	return nanoGPTXMLCall{name: name, arguments: string(encoded)}, true, nil
}
