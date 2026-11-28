package proxyhandler

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/deliciousbuding/metapi-go/internal/upstreamadapter"
)

const directNanoGPTMaxItems = 128
const directNanoGPTXMLLimit = 100_000

// Only standalone XML blocks can be calls. Inline examples, fenced/indented
// code and Markdown block quotes remain text. One entire unknown XML root is
// opaque, so a declared tool nested inside an ordinary document is not invoked.
type nanoGPTText struct {
	tools       *upstreamadapter.NanoGPTTools
	buffer      []byte
	plainLine   bool
	fence       byte
	fenceWidth  int
	inlineWidth int
	tickRun     int
	block       *nanoGPTXMLBlock
}

func newNanoGPTText(tools *upstreamadapter.NanoGPTTools) *nanoGPTText {
	return &nanoGPTText{tools: tools}
}

func (p *nanoGPTText) feed(text string, final bool) (string, []nanoGPTXMLCall, error) {
	if p.tools == nil || p.tools.Empty() {
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
				if part.err != nil && (p.tools.Allows(part.name) || part.name == "use_tool") {
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
