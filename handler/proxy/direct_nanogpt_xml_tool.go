package proxyhandler

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"strings"

	"github.com/deliciousbuding/metapi-go/internal/upstreamadapter"
)

type nanoGPTXMLCall struct{ name, arguments string }
type nanoGPTXMLNode struct {
	name     string
	attrs    map[string]string
	text     strings.Builder
	children []*nanoGPTXMLNode
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

func nanoGPTXMLTool(node *nanoGPTXMLNode, tools *upstreamadapter.NanoGPTTools) (nanoGPTXMLCall, bool, error) {
	if node == nil || tools == nil {
		return nanoGPTXMLCall{}, false, nil
	}
	name := node.name
	if name == "use_tool" {
		name = node.attrs["name"]
	}
	if !tools.Allows(name) {
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
