package store

import "testing"

func TestChannelPluginNativeProtocolsRemainExplicit(t *testing.T) {
	for _, protocol := range []string{"responses", "responsesCompact", "responsesWebsocket"} {
		config := DirectRequestConfig{Plugins: []DirectPluginBinding{{Key: "native", Protocols: []string{protocol}}}}
		endpoints := []DirectEndpoints{
			{Responses: &DirectEndpoint{URL: "https://example.invalid/responses", Auth: DirectAuthBearer}},
			{ResponsesCompact: &DirectEndpoint{URL: "https://example.invalid/compact", Auth: DirectAuthBearer}},
			{ResponsesWebsocket: &DirectEndpoint{URL: "wss://example.invalid/responses", Auth: DirectAuthBearer}},
		}
		for _, endpoint := range endpoints {
			key := ""
			for _, entry := range endpoint.Entries() {
				if entry.Endpoint != nil {
					key = entry.Key
				}
			}
			matching := key == protocol
			if err := config.ValidateEndpoints(endpoint); (err == nil) != matching {
				t.Errorf("binding=%s endpoint=%s err=%v", protocol, key, err)
			}
		}
		value, err := config.Value()
		if err != nil {
			t.Fatal(err)
		}
		var restored DirectRequestConfig
		if err := restored.Scan(value); err != nil || len(restored.Plugins) != 1 || restored.Plugins[0].Protocols[0] != protocol {
			t.Fatalf("protocol scope lost in storage: %+v %v", restored, err)
		}
	}
}
