package handler

import (
	"testing"

	"github.com/tigerowo/infinite-canvas/model"
)

// [CUSTOM] 上游的三条链路都会调用 prepareAIProtocolRequest，但 mode 不同：
//
//	aiProtocolProxyRequest  —— 账号渠道（登录后）走画布后端代理，画布图片节点也走这条
//	aiProtocolVideoRequest  —— 视频创作台建任务
//	aiProtocolDirectRequest —— 本地渠道的参数转译接口（浏览器拿计划后自己打上游）
//
// Fal / Replicate 的 prepare 钩子最初只在 direct 模式下生效，导致代理模式下把画布形态的
// 报文（model / size / n …）原样发给了两个平台，上游直接 400，且错误体是 FastAPI 风格的
// `{"detail": ...}`，又被 readUpstreamAIErrorMessage 忽略，最终只剩一句"AI 接口请求失败：400"。
//
// 本文件同时守住两件事：三种模式转译结果一致；平台错误体必须能被读出人话。
func TestModelProtocolProxyTranslatesLikeDirect(t *testing.T) {
	blockProtocolNetwork(t)
	tests := []struct {
		name, protocol, model, endpoint, body, want string
	}{
		{
			name: "fal text to image", protocol: "fal", model: "fal-ai/flux/dev?image_size=landscape_16_9",
			endpoint: "/images/generations",
			body:     `{"model":"fal-ai/flux/dev","prompt":"scene","size":"1024x1024","n":1}`,
			want:     `{"prompt":"scene","image_size":"landscape_16_9"}`,
		},
		{
			name: "fal image edit", protocol: "fal", model: "fal-ai/flux-pro/kontext",
			endpoint: "/images/edits",
			body:     `{"model":"fal-ai/flux-pro/kontext","prompt":"scene","image":"https://media.invalid/a.png","size":"1024x1024"}`,
			want:     `{"prompt":"scene","image_url":"https://media.invalid/a.png"}`,
		},
		{
			name: "fal video", protocol: "fal", model: "fal-ai/kling-video/v2.1/master/text-to-video",
			endpoint: "/videos",
			body:     `{"model":"fal-ai/kling-video/v2.1/master/text-to-video","prompt":"scene","seconds":"5","size":"1280x720"}`,
			want:     `{"prompt":"scene","aspect_ratio":"16:9","duration":"5"}`,
		},
		{
			name: "replicate official image", protocol: "replicate", model: "black-forest-labs/flux-1.1-pro",
			endpoint: "/images/generations",
			body:     `{"model":"black-forest-labs/flux-1.1-pro","prompt":"scene","n":1}`,
			want:     `{"input":{"prompt":"scene"}}`,
		},
		{
			name: "replicate video", protocol: "replicate", model: "acme/thing:abc123",
			endpoint: "/videos",
			body:     `{"model":"acme/thing:abc123","prompt":"scene","seconds":"5","size":"720x1280"}`,
			want:     `{"input":{"prompt":"scene","aspect_ratio":"9:16","duration":5},"version":"abc123"}`,
		},
	}
	modes := []struct {
		name string
		mode aiProtocolRequestMode
	}{
		{"proxy", aiProtocolProxyRequest},
		{"video", aiProtocolVideoRequest},
	}
	for _, mode := range modes {
		for _, test := range tests {
			t.Run(mode.name+"/"+test.name, func(t *testing.T) {
				prepared, provider, err := prepareAIProtocolRequest(aiProtocolRequest{
					mode:      mode.mode,
					body:      []byte(test.body),
					modelName: test.model,
					channel:   model.ModelChannel{Protocol: test.protocol, BaseURL: "https://upstream.invalid"},
					endpoint:  test.endpoint,
					path:      test.endpoint,
				})
				if err != nil {
					t.Fatal(err)
				}
				if provider != test.protocol {
					t.Fatalf("got provider %q; want %q", provider, test.protocol)
				}
				assertProtocolBytes(t, prepared.body, []byte(test.want))
			})
		}
	}
}

// 平台错误体形状与 OpenAI 完全不同，读不出来就等于没有错误信息。
func TestUpstreamErrorMessageReadsPlatformDetails(t *testing.T) {
	tests := []struct {
		name, body, want string
		status           int
	}{
		{name: "fal detail string", body: `{"detail":"Cannot access application \"fal-ai/flux\"."}`, want: `Cannot access application "fal-ai/flux".`},
		{name: "fal detail list", body: `{"detail":[{"loc":["body","image_size"],"msg":"unexpected value"}]}`, want: "unexpected value"},
		{name: "fal detail list without msg", body: `{"detail":[{"type":"missing","loc":["body","prompt"]}]}`, want: "missing: body.prompt"},
		{name: "fal detail list caps length", body: `{"detail":[{"msg":"a"},{"msg":"b"},{"msg":"c"},{"msg":"d"}]}`, want: "a；b；c"},
		{name: "replicate detail", body: `{"status":500,"detail":"Internal server error"}`, want: "Internal server error"},
		{name: "fal plain text", body: `Unauthorized`, want: "Unauthorized"},
		{name: "openai unchanged", body: `{"error":{"message":"bad size","type":"openai_error"}}`, want: "bad size"},
		{name: "new api unchanged", body: `{"msg":"额度不足"}`, want: "额度不足"},
		{name: "html page still opaque", body: `<html><body>Bad Gateway</body></html>`, want: "AI 接口请求失败：502", status: 502},
		{name: "empty still opaque", body: ``, want: "AI 接口请求失败：400"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			status := test.status
			if status == 0 {
				status = 400
			}
			if got := readUpstreamAIErrorMessage([]byte(test.body), status); got != test.want {
				t.Fatalf("got %q; want %q", got, test.want)
			}
		})
	}
}
