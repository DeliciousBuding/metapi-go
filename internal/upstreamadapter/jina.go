package upstreamadapter

import (
	"encoding/json"
	"fmt"
)

func prepareJinaEmbeddings(body []byte) ([]byte, error) {
	var obj map[string]json.RawMessage
	if json.Unmarshal(body, &obj) != nil || obj == nil {
		return nil, fmt.Errorf("invalid Jina embeddings request")
	}
	if _, ok := obj["task"]; ok {
		return body, nil
	}
	obj["task"] = json.RawMessage(`"text-matching"`)
	return json.Marshal(obj)
}
