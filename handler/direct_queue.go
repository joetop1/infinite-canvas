package handler

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/tigerowo/infinite-canvas/model"
	"github.com/tigerowo/infinite-canvas/service"
)

// [CUSTOM] Fal.ai / Replicate 都是"提交拿任务 ID + 轮询取结果"的队列型平台，而画布后端
// 的同步接口（/api/v1/images/generations、/api/v1/videos）是 OpenAI 形态的。
// 这里提供两者共用的队列工具：轮询循环、取结果、把产物拼成 OpenAI 形态的响应。
//
// 参照上游既有的 APIMart / KIE 处理方式（handler/apimart_image.go 的 pollAPIMartImageTask）：
// 在 copyResponse 钩子里替用户把队列跑完，再回一个 OpenAI 形态的响应，
// 这样画布节点、图片工作台、视频创作台都不需要知道上游是队列模型。
const (
	directQueuePollAttempts = 300
	directQueuePollInterval = 2 * time.Second
	directQueueReadLimit    = 512 * 1024
)

// isDirectImageEndpoint 判断画布侧接口是不是图片生成。
func isDirectImageEndpoint(endpoint string) bool {
	return endpoint == "/images/generations" || endpoint == "/images/edits"
}

// directTaskIDFromPath 从画布的任务轮询路径 /videos/{id} 中取出上游任务 ID，
// 供两个平台拼各自的查询地址（Fal 是 /requests/{id}/status，Replicate 是 /predictions/{id}）。
func directTaskIDFromPath(path string) (string, bool) {
	if !strings.HasPrefix(path, "/videos/") || strings.HasSuffix(path, "/content") {
		return "", false
	}
	id := strings.TrimSpace(strings.TrimPrefix(path, "/videos/"))
	if id == "" || strings.Contains(id, "/") {
		return "", false
	}
	return id, true
}

// pollDirectQueue 按固定间隔轮询，直到 readPoll 报告完成、报错、或超时。
// readPoll 返回 (done, errorMessage)；两者都空表示"仍在处理"。
func pollDirectQueue(request *http.Request, channel model.ModelChannel, target string, readPoll func([]byte) (bool, string)) ([]byte, string) {
	for attempt := 0; attempt < directQueuePollAttempts; attempt++ {
		if attempt > 0 {
			select {
			case <-requestContext(request).Done():
				return nil, requestContext(request).Err().Error()
			case <-time.After(directQueuePollInterval):
			}
		}
		payload, status, err := directQueueGET(request, channel, target)
		if err != nil {
			return nil, err.Error()
		}
		if status >= http.StatusBadRequest {
			return nil, readUpstreamAIErrorMessage(payload, status)
		}
		done, message := readPoll(payload)
		if message != "" {
			return nil, message
		}
		if done {
			return payload, ""
		}
	}
	return nil, "上游队列任务超时，请稍后在创作台查看结果"
}

func directQueueGET(request *http.Request, channel model.ModelChannel, target string) ([]byte, int, error) {
	pollRequest, err := http.NewRequestWithContext(requestContext(request), http.MethodGet, target, nil)
	if err != nil {
		return nil, 0, err
	}
	service.SetModelChannelAuthHeader(pollRequest, channel)
	response, err := service.HTTPClientForChannel(channel).Do(pollRequest)
	if err != nil {
		return nil, 0, err
	}
	defer response.Body.Close()
	payload, _ := io.ReadAll(io.LimitReader(response.Body, directQueueReadLimit))
	return payload, response.StatusCode, nil
}

func requestContext(request *http.Request) context.Context {
	if request == nil {
		return context.Background()
	}
	return request.Context()
}

// sameHostQueueURL 只接受与渠道同源的上游地址。
// 上游返回的 URL 会被当作下一步请求目标，限制同源可以避免渠道被配置成恶意的跳板。
func sameHostQueueURL(candidate string, channel model.ModelChannel) string {
	candidate = strings.TrimSpace(candidate)
	if candidate == "" {
		return ""
	}
	target, err := url.Parse(candidate)
	if err != nil || target.Host == "" || (target.Scheme != "http" && target.Scheme != "https") {
		return ""
	}
	base, err := url.Parse(strings.TrimSpace(channel.BaseURL))
	if err != nil || !strings.EqualFold(base.Scheme, target.Scheme) || !strings.EqualFold(base.Host, target.Host) {
		return ""
	}
	return target.String()
}

// collectDirectURLs 只沿已知字段名递归，避免把 prompt、状态地址等普通字符串误当成产物地址。
func collectDirectURLs(value any, keys []string, depth int) []string {
	if depth > 6 || value == nil {
		return nil
	}
	switch typed := value.(type) {
	case string:
		text := strings.TrimSpace(typed)
		if strings.HasPrefix(text, "http://") || strings.HasPrefix(text, "https://") {
			return []string{text}
		}
	case []any:
		var result []string
		for _, item := range typed {
			result = append(result, collectDirectURLs(item, keys, depth+1)...)
		}
		return result
	case map[string]any:
		var result []string
		for _, key := range keys {
			result = append(result, collectDirectURLs(typed[key], keys, depth+1)...)
		}
		return result
	}
	return nil
}

func uniqueHTTPURLs(values []string) []string {
	seen := map[string]bool{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		result = append(result, value)
	}
	return result
}

func decodeDirectQueuePayload(payload []byte) map[string]any {
	root := map[string]any{}
	if len(payload) == 0 || json.Unmarshal(payload, &root) != nil {
		return map[string]any{}
	}
	return root
}

// readDirectMediaURLs 从上游产物报文里取出媒体地址；报文是数组也能处理。
func readDirectMediaURLs(payload []byte, keys []string) []string {
	var root any
	if len(payload) == 0 || json.Unmarshal(payload, &root) != nil {
		return nil
	}
	return uniqueHTTPURLs(collectDirectURLs(root, keys, 0))
}

func marshalDirectMap(value map[string]any) ([]byte, bool) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, false
	}
	return encoded, true
}

func readDirectStringField(root map[string]any, key string) string {
	return strings.TrimSpace(toStringSafe(root[key]))
}

// writeDirectImagesResponse 把产物地址包装成画布认识的 OpenAI 图片响应。
func writeDirectImagesResponse(w http.ResponseWriter, statusCode int, imageURLs []string, logContext aiLogContext) {
	items := make([]map[string]any, 0, len(imageURLs))
	for _, imageURL := range imageURLs {
		items = append(items, map[string]any{"url": imageURL})
	}
	encoded, err := json.Marshal(map[string]any{"created": time.Now().Unix(), "data": items})
	if err != nil {
		writeDirectImageError(w, statusCode, err.Error(), logContext)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_, _ = w.Write(encoded)
	saveAIProxyLog(logContext, statusCode, string(encoded), "")
}

func writeDirectImageError(w http.ResponseWriter, statusCode int, message string, logContext aiLogContext) {
	if statusCode < http.StatusBadRequest {
		statusCode = http.StatusBadGateway
	}
	body, _ := json.Marshal(map[string]any{"error": map[string]any{"message": message}})
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_, _ = w.Write(body)
	saveAIProxyLog(logContext, statusCode, string(body), message)
}

// writeDirectRawResponse 原样回传上游报文（用于"看起来不是队列提交"的情况）。
func writeDirectRawResponse(w http.ResponseWriter, response *http.Response, payload []byte, logContext aiLogContext) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(response.StatusCode)
	_, _ = w.Write(payload)
	saveAIProxyLog(logContext, response.StatusCode, string(payload), "")
}
