package proxy

import "testing"

func TestCompletionImageOutputEvidence(t *testing.T) {
	for _, body := range []string{
		`{"choices":[{"message":{"content":[{"type":"image_url","image_url":{"url":"data:image/png;base64,AA=="}}]}}]}`,
		`{"choices":[{"message":{"images":[{"type":"image_url","image_url":{"url":"https://images.example/image.png"}}]}}]}`,
		`{"choices":[{"delta":{"content":[{"type":"image_url","image_url":{"url":"data:image/png;base64,AA=="}}]}}]}`,
		`{"choices":[{"delta":{"images":[{"type":"image_url","image_url":{"url":"https://images.example/image.png"}}]}}]}`,
		`{"candidates":[{"content":{"parts":[{"inlineData":{"mimeType":"image/png","data":"AA=="}}]}}]}`,
		`{"candidates":[{"content":{"parts":[{"fileData":{"mimeType":"image/png","fileUri":"https://images.example/image.png"}}]}}]}`,
	} {
		if !detectHasUpstreamOutput(body) {
			t.Errorf("image ignored: %s", body)
		}
	}
	for _, body := range []string{
		`{"choices":[{"message":{"content":[],"images":[]}}]}`,
		`{"choices":[{"delta":{"images":[{"type":"image_url","image_url":{"url":""}}]}}]}`,
		`{"choices":[{"message":{"content":[{"type":"image_url"}]}}]}`,
		`{"candidates":[{"content":{"parts":[{"inlineData":{"mimeType":"image/png"}}]}}]}`,
		`{"candidates":[{"content":{"parts":[{"fileData":{"mimeType":"image/png","fileUri":""}}]}}]}`,
	} {
		if detectHasUpstreamOutput(body) {
			t.Errorf("empty image metadata counted as output: %s", body)
		}
	}
}
