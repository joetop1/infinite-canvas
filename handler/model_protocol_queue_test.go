package handler

import (
	"bytes"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/tigerowo/infinite-canvas/model"
)

// 账号渠道（登录后）下，画布的图片请求打到画布后端，上游回的是"已入队"。
// 这组测试用脚本化的上游，验证后端会替用户把队列跑完并给出 OpenAI 形态的响应。
func TestFalQueueImageProxyFlow(t *testing.T) {
	const submit = `{"request_id":"req-1","status_url":"https://upstream.invalid/fal-ai/flux/requests/req-1/status","response_url":"https://upstream.invalid/fal-ai/flux/requests/req-1/response","queue_position":0}`
	channel := model.ModelChannel{Protocol: "fal", BaseURL: "https://upstream.invalid"}

	tests := []struct {
		name    string
		submit  string
		steps   []protocolStep
		want    string
		wantErr string
	}{
		{
			name: "completed returns openai images response",
			steps: []protocolStep{
				{"GET", "https://upstream.invalid/fal-ai/flux/requests/req-1/status", 200, `{"status":"COMPLETED"}`},
				{"GET", "https://upstream.invalid/fal-ai/flux/requests/req-1/response", 200, `{"images":[{"url":"https://media.invalid/a.png","width":1024}],"seed":42}`},
			},
			want: `{"data":[{"url":"https://media.invalid/a.png"}]}`,
		},
		{
			name: "queue path drops model sub path",
			// 模型是三段 fal-ai/flux/dev，队列路径必须只取前两段，否则上游回 405。
			// 这里刻意不带 status_url / response_url，逼代码自己拼地址。
			submit: `{"request_id":"req-1","queue_position":0}`,
			steps: []protocolStep{
				{"GET", "https://upstream.invalid/fal-ai/flux/requests/req-1/status", 200, `{"status":"COMPLETED"}`},
				{"GET", "https://upstream.invalid/fal-ai/flux/requests/req-1/response", 200, `{"images":[{"url":"https://media.invalid/a.png"}]}`},
			},
			want: `{"data":[{"url":"https://media.invalid/a.png"}]}`,
		},
		{
			name: "failed task surfaces platform error",
			steps: []protocolStep{
				{"GET", "https://upstream.invalid/fal-ai/flux/requests/req-1/status", 200, `{"status":"COMPLETED","error":"NSFW content detected","error_type":"content_policy_violation"}`},
			},
			wantErr: "NSFW content detected",
		},
		{
			name: "completed without image is explicit",
			steps: []protocolStep{
				{"GET", "https://upstream.invalid/fal-ai/flux/requests/req-1/status", 200, `{"status":"COMPLETED"}`},
				{"GET", "https://upstream.invalid/fal-ai/flux/requests/req-1/response", 200, `{"prompt":"scene"}`},
			},
			wantErr: "Fal 任务已完成但没有返回图片地址",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			protocolScript(t, test.steps)
			submitted := test.submit
			if submitted == "" {
				submitted = submit
			}
			recorder := httptest.NewRecorder()
			response := &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(submitted))}
			request := httptest.NewRequest(http.MethodPost, "/api/v1/images/generations", nil)
			logContext := aiLogContext{Endpoint: "/images/generations", Model: "fal-ai/flux/dev", Channel: channel}
			if !copyFalImageResponse(recorder, response, request, channel, logContext, nil) {
				t.Fatal("fal image response was not handled")
			}
			if test.wantErr != "" {
				if recorder.Code < http.StatusBadRequest {
					t.Fatalf("got status %d; want an error", recorder.Code)
				}
				if !strings.Contains(recorder.Body.String(), test.wantErr) {
					t.Fatalf("got %s; want it to contain %q", recorder.Body.String(), test.wantErr)
				}
				return
			}
			assertDirectImageData(t, recorder.Body.String(), test.want)
		})
	}
}

// 上游自报的地址只接受同源，避免渠道被配置成把请求转去别处。
func TestFalQueueIgnoresForeignHostURLs(t *testing.T) {
	protocolScript(t, []protocolStep{
		{"GET", "https://upstream.invalid/fal-ai/flux/requests/req-1/status", 200, `{"status":"COMPLETED"}`},
		{"GET", "https://upstream.invalid/fal-ai/flux/requests/req-1/response", 200, `{"images":[{"url":"https://media.invalid/a.png"}]}`},
	})
	channel := model.ModelChannel{Protocol: "fal", BaseURL: "https://upstream.invalid"}
	if got := sameHostQueueURL("http://upstream.invalid/fal-ai/flux/requests/req-1/status", channel); got != "" {
		t.Fatalf("accepted a URL that downgrades the channel scheme: %q", got)
	}
	submit := `{"request_id":"req-1","status_url":"https://evil.invalid/steal","response_url":"https://evil.invalid/steal"}`

	recorder := httptest.NewRecorder()
	response := &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(submit))}
	request := httptest.NewRequest(http.MethodPost, "/api/v1/images/generations", nil)
	if !copyFalImageResponse(recorder, response, request, channel, aiLogContext{Endpoint: "/images/generations", Model: "fal-ai/flux/dev", Channel: channel}, nil) {
		t.Fatal("fal image response was not handled")
	}
	assertDirectImageData(t, recorder.Body.String(), `{"data":[{"url":"https://media.invalid/a.png"}]}`)
}

func TestFalVideoProxyFlow(t *testing.T) {
	channel := model.ModelChannel{Protocol: "fal", BaseURL: "https://upstream.invalid"}
	const model_ = "fal-ai/kling-video/v2.1/master/text-to-video"

	// 轮询地址：嵌套模型 ID 同样只取前两段。
	if path := resolveAIProxyPath(channel, model_, "/videos/req-9"); path != "/fal-ai/kling-video/requests/req-9/status" {
		t.Fatalf("got poll path %q", path)
	}

	created := transformAIProtocolVideoPayload([]byte(`{"request_id":"req-9","status_url":"https://upstream.invalid/fal-ai/kling-video/requests/req-9/status"}`), nil, channel, model_, false)
	assertProtocolJSONValue(t, protocolJSON(t, string(created)), `{"id":"req-9","task_id":"req-9","status":"processing","progress":0}`)

	statusRequest := httptest.NewRequest(http.MethodGet, "https://upstream.invalid/fal-ai/kling-video/requests/req-9/status", nil)

	protocolScript(t, []protocolStep{
		{"GET", "https://upstream.invalid/fal-ai/kling-video/requests/req-9/response", 200, `{"video":{"url":"https://media.invalid/v.mp4"}}`},
	})
	completed := transformAIProtocolVideoPayload([]byte(`{"status":"COMPLETED"}`), statusRequest, channel, model_, true)
	assertProtocolJSONValue(t, protocolJSON(t, string(completed)), `{"status":"completed","progress":100,"video_url":"https://media.invalid/v.mp4","url":"https://media.invalid/v.mp4"}`)

	pending := transformAIProtocolVideoPayload([]byte(`{"status":"IN_QUEUE"}`), statusRequest, channel, model_, true)
	assertProtocolJSONValue(t, protocolJSON(t, string(pending)), `{"status":"processing"}`)

	failed := transformAIProtocolVideoPayload([]byte(`{"status":"COMPLETED","error_type":"nsfw"}`), statusRequest, channel, model_, true)
	assertProtocolJSONValue(t, protocolJSON(t, string(failed)), `{"status":"failed","error":"Fal 任务失败（nsfw）"}`)

	if message := readAIProtocolVideoError([]byte(`{"status":"COMPLETED","error":"boom"}`), channel, model_, true); message != "boom" {
		t.Fatalf("got error message %q", message)
	}
}

func TestReplicateProxyFlow(t *testing.T) {
	channel := model.ModelChannel{Protocol: "replicate", BaseURL: "https://upstream.invalid"}

	if path := resolveAIProxyPath(channel, "black-forest-labs/flux-1.1-pro", "/videos/pred-1"); path != "/predictions/pred-1" {
		t.Fatalf("got poll path %q", path)
	}
	if path := resolveAIProxyPath(channel, "acme/thing:abc123", "/videos/pred-2"); path != "/predictions/pred-2" {
		t.Fatalf("got poll path %q", path)
	}

	created := transformAIProtocolVideoPayload([]byte(`{"id":"pred-1","status":"starting","urls":{"get":"https://upstream.invalid/v1/predictions/pred-1"}}`), nil, channel, "black-forest-labs/flux-1.1-pro", false)
	assertProtocolJSONValue(t, protocolJSON(t, string(created)), `{"id":"pred-1","task_id":"pred-1","status":"processing","progress":0}`)

	// 轮询报文里的 urls.get 是接口地址，不能当成产物地址。
	succeeded := transformAIProtocolVideoPayload([]byte(`{"id":"pred-1","status":"succeeded","urls":{"get":"https://upstream.invalid/v1/predictions/pred-1"},"output":"https://media.invalid/v.mp4"}`), nil, channel, "black-forest-labs/flux-1.1-pro", true)
	assertProtocolJSONValue(t, protocolJSON(t, string(succeeded)), `{"status":"completed","progress":100,"video_url":"https://media.invalid/v.mp4","url":"https://media.invalid/v.mp4"}`)

	failed := transformAIProtocolVideoPayload([]byte(`{"id":"pred-1","status":"failed","error":"NSFW"}`), nil, channel, "black-forest-labs/flux-1.1-pro", true)
	assertProtocolJSONValue(t, protocolJSON(t, string(failed)), `{"status":"failed","error":"NSFW"}`)
	aborted := transformAIProtocolVideoPayload([]byte(`{"id":"pred-1","status":"aborted"}`), nil, channel, "black-forest-labs/flux-1.1-pro", true)
	assertProtocolJSONValue(t, protocolJSON(t, string(aborted)), `{"status":"failed","error":"Replicate 任务已中止"}`)
	if done, message := readReplicateProgress([]byte(`{"status":"aborted"}`)); done || message != "Replicate 任务已中止" {
		t.Fatalf("aborted prediction must report a terminal error: done=%v message=%q", done, message)
	}

	protocolScript(t, []protocolStep{
		{"GET", "https://upstream.invalid/v1/predictions/pred-1", 200, `{"id":"pred-1","status":"succeeded","urls":{"get":"https://upstream.invalid/v1/predictions/pred-1"},"output":["https://media.invalid/a.png"]}`},
	})
	recorder := httptest.NewRecorder()
	response := &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(`{"id":"pred-1","status":"starting","urls":{"get":"https://upstream.invalid/v1/predictions/pred-1"}}`))}
	request := httptest.NewRequest(http.MethodPost, "/api/v1/images/generations", nil)
	if !copyReplicateImageResponse(recorder, response, request, channel, aiLogContext{Endpoint: "/images/generations", Model: "black-forest-labs/flux-1.1-pro", Channel: channel}, nil) {
		t.Fatal("replicate image response was not handled")
	}
	assertDirectImageData(t, recorder.Body.String(), `{"data":[{"url":"https://media.invalid/a.png"}]}`)
}

// assertDirectImageData 只比对 data 字段，created 是当前时间戳。
func assertDirectImageData(t *testing.T, body string, want string) {
	t.Helper()
	root, ok := protocolJSON(t, body).(map[string]any)
	if !ok {
		t.Fatalf("unexpected image response: %s", body)
	}
	delete(root, "created")
	assertProtocolJSONValue(t, root, want)
}

// 画布在有参考图时发的是 multipart，图片在文件字段里；转译必须能把它们变成 data URI。
func TestDirectProxyMultipartReferences(t *testing.T) {
	body := directProxyMultipartBody(t, "/images/edits", "image", "reference.png", "image/png", []byte("\x89PNG-fake-bytes"))
	prepared, provider, err := prepareAIProtocolRequest(aiProtocolRequest{
		mode: aiProxyRequestMode(), body: body, contentType: "multipart/form-data; boundary=" + directProxyBoundary,
		modelName: "fal-ai/flux-pro/kontext", endpoint: "/images/edits", path: "/images/edits",
		channel: model.ModelChannel{Protocol: "fal", BaseURL: "https://upstream.invalid"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if provider != "fal" {
		t.Fatalf("got provider %q", provider)
	}
	translated := protocolJSON(t, string(prepared.body)).(map[string]any)
	imageURL, _ := translated["image_url"].(string)
	if !strings.HasPrefix(imageURL, "data:image/png;base64,") {
		t.Fatalf("reference was not inlined as a data URI: %v", translated)
	}
	if _, leaked := translated["_canvas_endpoint"]; leaked {
		t.Fatalf("canvas meta fields leaked upstream: %v", translated)
	}
}

const directProxyBoundary = "canvas-boundary-1234"

func directProxyMultipartBody(t *testing.T, endpoint string, fileField string, filename string, contentType string, content []byte) []byte {
	t.Helper()
	buffer := &bytes.Buffer{}
	writer := multipart.NewWriter(buffer)
	_ = writer.SetBoundary(directProxyBoundary)
	_ = writer.WriteField("_canvas_endpoint", endpoint)
	_ = writer.WriteField("prompt", "scene")
	part, err := writer.CreateFormFile(fileField, filename)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write(content); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	_ = contentType
	return buffer.Bytes()
}

// 画布实际用的是 aiProtocolProxyRequest；单独抽出来是为了让上面的用例读起来清楚。
func aiProxyRequestMode() aiProtocolRequestMode { return aiProtocolProxyRequest }

type protocolStep struct {
	method string
	url    string
	status int
	body   string
}

// protocolScript 按顺序返回预置响应；出现脚本之外的请求立即失败。
func protocolScript(t *testing.T, steps []protocolStep) {
	t.Helper()
	index := 0
	protocolMockHTTP(t, func(request *http.Request) (*http.Response, error) {
		if index >= len(steps) {
			t.Errorf("unexpected upstream request: %s %s", request.Method, request.URL)
			return nil, errors.New("no scripted response left")
		}
		step := steps[index]
		index++
		if request.Method != step.method || request.URL.String() != step.url {
			t.Errorf("step %d: got %s %s; want %s %s", index, request.Method, request.URL, step.method, step.url)
		}
		return &http.Response{
			StatusCode: step.status,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(step.body)),
		}, nil
	})
}
