package handler

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/tigerowo/infinite-canvas/model"
)

func TestOpenRouterVideoMultipart(t *testing.T) {
	blockProtocolNetwork(t)
	var body bytes.Buffer
	form := multipart.NewWriter(&body)
	for key, value := range map[string]string{"model": "google/veo-3.1", "prompt": "moonlit village", "seconds": "4", "size": "1280x720", "resolution_name": "720p", "preset": "normal", "input_reference[]": "https://media.invalid/reference.png", "video_generate_audio": "false"} {
		if err := form.WriteField(key, value); err != nil {
			t.Fatal(err)
		}
	}
	frame, err := form.CreateFormFile("first_frame_url", "start.png")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = frame.Write([]byte("image")); err != nil {
		t.Fatal(err)
	}
	if err = form.Close(); err != nil {
		t.Fatal(err)
	}
	for _, mode := range []aiProtocolRequestMode{aiProtocolProxyRequest, aiProtocolVideoRequest, aiProtocolDirectRequest} {
		got, _, err := prepareAIProtocolRequest(aiProtocolRequest{mode: mode, channel: model.ModelChannel{Protocol: "openai", BaseURL: "https://openrouter.ai/api/v1"}, endpoint: "/videos", path: "/videos", modelName: "google/veo-3.1", body: body.Bytes(), contentType: form.FormDataContentType()})
		if err != nil {
			t.Fatal(err)
		}
		if got.contentType != "application/json" {
			t.Fatalf("content type = %q", got.contentType)
		}
		assertProtocolBytes(t, got.body, []byte(`{"model":"google/veo-3.1","prompt":"moonlit village","duration":4,"size":"1280x720","resolution":"720p","generate_audio":false,"input_references":[{"type":"image_url","image_url":{"url":"https://media.invalid/reference.png"}}],"frame_images":[{"type":"image_url","image_url":{"url":"data:image/png;base64,aW1hZ2U="},"frame_type":"first_frame"}]}`))
	}
}

func TestOpenRouterVideoScopeAndJSON(t *testing.T) {
	blockProtocolNetwork(t)
	raw := []byte(`{"model":"google/veo-3.1","prompt":"scene","duration":8,"frame_images":[{"type":"image_url","image_url":{"url":"https://media.invalid/a.png"},"frame_type":"first_frame"}],"provider":{"options":{}}}`)
	input := aiProtocolRequest{channel: model.ModelChannel{Protocol: "openai", BaseURL: "https://openrouter.ai/api/v1"}, endpoint: "/videos", modelName: "google/veo-3.1", body: raw, contentType: "application/json"}
	got, handled, err := prepareOpenRouterVideoRequest(input)
	if err != nil || !handled {
		t.Fatalf("handled=%v err=%v", handled, err)
	}
	assertProtocolBytes(t, got.body, raw)
	for _, host := range []string{"https://api.openai.com/v1", "https://openrouter.ai.evil.invalid/v1"} {
		input.channel.BaseURL = host
		got, handled, err = prepareOpenRouterVideoRequest(input)
		if err != nil || handled || !bytes.Equal(got.body, raw) {
			t.Fatalf("other channel changed: %s", host)
		}
	}
}

func TestFalVideoUsesReturnedResultURL(t *testing.T) {
	channel := model.ModelChannel{Protocol: "fal", BaseURL: "https://upstream.invalid"}
	request := httptest.NewRequest(http.MethodGet, "https://upstream.invalid/fal-ai/kling-video/requests/req-9/status?logs=1", nil)
	for _, test := range []struct{ name, responseURL, target string }{
		{"returned same origin", "https://upstream.invalid/custom/result/req-9", "https://upstream.invalid/custom/result/req-9"},
		{"foreign origin rejected", "https://evil.invalid/steal", "https://upstream.invalid/fal-ai/kling-video/requests/req-9"},
	} {
		t.Run(test.name, func(t *testing.T) {
			protocolScript(t, []protocolStep{{"GET", test.target, 200, `{"video":{"url":"https://media.invalid/video.mp4"}}`}})
			got, message := fetchFalVideoURL(request, channel, map[string]any{"response_url": test.responseURL})
			if got != "https://media.invalid/video.mp4" || message != "" {
				t.Fatalf("url=%q error=%q", got, message)
			}
		})
	}
}

func TestReplicateWanRejectsMultipleImages(t *testing.T) {
	for _, raw := range []string{`{"input_reference[]":["https://media.invalid/1.png","https://media.invalid/2.png"]}`, `{}`, `{"input_reference[]":"https://media.invalid/1.png","first_frame_url":"https://media.invalid/2.png"}`} {
		_, err := normalizeReplicateDirectBody([]byte(raw), "application/json", "wan-video/wan-2.5-i2v", "/videos")
		if err == nil || !strings.Contains(err.Error(), "一张") {
			t.Fatalf("expected local image validation, got %v", err)
		}
	}
	got, err := normalizeReplicateDirectBody([]byte(`{"prompt":"scene","first_frame_url":"https://media.invalid/1.png","seconds":"10"}`), "application/json", "wan-video/wan-2.5-i2v", "/videos")
	if err != nil {
		t.Fatal(err)
	}
	assertProtocolBytes(t, got, []byte(`{"input":{"prompt":"scene","image":"https://media.invalid/1.png","duration":10}}`))
}
