package messages

import "encoding/json"

// PrepareVertexRequest adapts a native Messages body for Anthropic on Vertex:
// the model selector moves to the publisher URL, and the Anthropic version
// rides in the body as anthropic_version. stream stays — the URL action
// (rawPredict vs streamRawPredict) is derived from it and they always agree.
// Every other native field passes through untouched.
func PrepareVertexRequest(body []byte) ([]byte, error) {
	value, err := object(body, "Vertex request")
	if err != nil {
		return nil, err
	}
	delete(value, "model")
	value["anthropic_version"] = json.RawMessage(`"vertex-2023-10-16"`)
	return json.Marshal(value)
}
