package handler

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/tigerowo/infinite-canvas/model"
	"github.com/tigerowo/infinite-canvas/service"
)

// Fal 队列接口的报文转译。
//
// 画布提交的是 OpenAI 形态的参数（prompt / size / n / image...），而 Fal 每个模型的输入
// schema 各不相同（fal-ai/flux/dev 用 image_size、kling 用 aspect_ratio + duration…），
// 后端无法穷举。因此这里只映射"几乎所有模型都认"的通用字段：
//
//	prompt / num_images / aspect_ratio / duration / 参考素材字段
//
// 其余模型专属参数通过模型名后的 query 传入，例如
// `fal-ai/kling-video/v2.1/master/text-to-video?duration=10&negative_prompt=blur`。
func normalizeFalDirectBody(raw []byte, contentType string, modelName string, endpoint string) ([]byte, error) {
	body, err := decodeDirectRequestBody(raw, contentType, "Fal")
	if err != nil {
		return nil, err
	}
	spec := parseDirectModelSpec(modelName)
	if spec.ModelPath == "" {
		return nil, errors.New("缺少 Fal 模型路径，请在模型名中填写如 fal-ai/flux/dev")
	}

	out := map[string]any{}
	if prompt := readDirectString(body["prompt"]); prompt != "" {
		out["prompt"] = prompt
	}

	switch endpoint {
	case "/images/generations":
		if count, ok := readDirectCount(body["n"]); ok {
			out["num_images"] = count
		}
	case "/images/edits":
		images := readDirectReferences(body, "image")
		if len(images) == 0 {
			return nil, errors.New("Fal 图片编辑需要至少一张参考图")
		}
		applyDirectMediaField(out, spec, images, "image_url", "image_urls", directImageFieldKey, directImagePluralKey)
		if count, ok := readDirectCount(body["n"]); ok {
			out["num_images"] = count
		}
	case "/videos":
		images := readDirectReferences(body, "input_reference[]")
		if len(images) == 0 {
			images = readDirectReferences(body, "first_frame_url")
		}
		applyDirectMediaField(out, spec, images, "image_url", "image_urls", directImageFieldKey, directImagePluralKey)
		videos := readDirectReferences(body, "video_reference[]")
		applyDirectMediaField(out, spec, videos, "video_url", "video_urls", directVideoFieldKey, directVideoPluralKey)
		if len(readDirectReferences(body, "audio_reference[]")) > 0 {
			return nil, errors.New("Fal 渠道暂不支持参考音频，请改用参考图片或参考视频")
		}
		// Fal 的视频模型普遍把时长声明为字符串枚举，这里保持字符串。
		if ratio := directAspectRatio(body["size"]); ratio != "" {
			out["aspect_ratio"] = ratio
		}
		if duration := readDirectString(body["seconds"]); duration != "" {
			out["duration"] = duration
		}
	default:
		return nil, errors.New("Fal 渠道不支持该接口")
	}

	applyDirectModelParams(out, spec.Params)
	return json.Marshal(out)
}

func prepareFalRequest(input aiProtocolRequest) (aiProtocolRequest, bool, error) {
	if !service.IsFalChannel(input.channel) || !isFalEndpoint(input.endpoint) {
		return input, false, nil
	}
	input.failureLabel = "Fal"
	body, err := normalizeFalDirectBody(input.body, input.contentType, input.modelName, input.endpoint)
	if err != nil {
		return input, true, err
	}
	input.body = body
	input.contentType = "application/json"
	return input, true, nil
}

// isFalEndpoint 只认画布上这三个创作接口。
//
// 上游的 prepare 钩子在三种 mode 下都会被调用（代理 / 视频任务 / 本地参数转译），
// 这里不按 mode 分流：账号渠道（登录后）走的是后端代理，本地直连走的是参数转译，
// 两条路都必须拿到同一份转译结果。Fal 没有对话模型，其余接口不认领即可，
// 让请求按原样去上游报错，比在这里造一个新错误分支更容易排查。
func isFalEndpoint(endpoint string) bool {
	switch endpoint {
	case "/images/generations", "/images/edits", "/videos":
		return true
	default:
		return false
	}
}

// falQueuePath 取模型 ID 的前两段（owner/app）作为队列路径。
//
// ★ 这里踩过坑：fal 的队列是按"应用"划分的，多段模型 ID 只有前两段是应用名，
// 其余是应用内的端点（见 fal 的 submit(path=...) 参数说明）。
// 用完整模型路径去请求队列接口会得到 405（路由不存在），实测对照：
//
//	GET queue.fal.run/fal-ai/flux/requests/bogus/status          → 404 {"status":"NOT_FOUND"}
//	GET queue.fal.run/fal-ai/flux/dev/requests/bogus/status      → 405（多了一段）
//	GET queue.fal.run/fal-ai/kling-video/requests/bogus/status   → 404（嵌套模型同样只取两段）
func falQueuePath(modelName string) string {
	segments := strings.Split(strings.Trim(parseDirectModelSpec(modelName).ModelPath, "/"), "/")
	if len(segments) < 2 || segments[0] == "" || segments[1] == "" {
		return ""
	}
	return segments[0] + "/" + segments[1]
}

// falQueueActionURL 拼出队列的 status / response 地址。
func falQueueActionURL(channel model.ModelChannel, modelName string, requestID string, action string) string {
	queue := falQueuePath(modelName)
	if queue == "" || strings.TrimSpace(requestID) == "" {
		return ""
	}
	return service.BuildModelChannelURL(channel, "/"+queue+"/requests/"+url.PathEscape(requestID)+"/"+action)
}

// falUpstreamPath 用模型路径作为提交地址：POST {base}/{modelId}；
// 轮询则走 {base}/{owner}/{app}/requests/{id}/status。
func falUpstreamPath(modelName string, path string) (string, bool) {
	if isFalEndpoint(path) {
		modelID := parseDirectModelSpec(modelName).ModelPath
		if modelID == "" {
			return path, true
		}
		return "/" + modelID, true
	}
	if taskID, ok := directTaskIDFromPath(path); ok {
		if queue := falQueuePath(modelName); queue != "" {
			return "/" + queue + "/requests/" + url.PathEscape(taskID) + "/status", true
		}
	}
	return path, true
}

// falRequestIDFromPayload 读取队列提交响应里的 request_id；
// 不带 request_id 的（少见）从它返回的 status_url / response_url 里兜底解析。
func falRequestIDFromPayload(payload []byte) string {
	root := decodeDirectQueuePayload(payload)
	if id := firstNonEmpty(readDirectStringField(root, "request_id"), readDirectStringField(root, "requestId")); id != "" {
		return id
	}
	for _, key := range []string{"response_url", "status_url"} {
		if id := falRequestIDFromURL(readDirectStringField(root, key)); id != "" {
			return id
		}
	}
	return ""
}

func falRequestIDFromURL(target string) string {
	parsed, err := url.Parse(strings.TrimSpace(target))
	if err != nil {
		return ""
	}
	segments := strings.Split(strings.Trim(parsed.Path, "/"), "/")
	for index, segment := range segments {
		if segment == "requests" && index+1 < len(segments) {
			return segments[index+1]
		}
	}
	return ""
}

// readFalQueueProgress 解读 status 地址的响应：完成、失败、还是仍在排队。
func readFalQueueProgress(payload []byte) (bool, string) {
	root := decodeDirectQueuePayload(payload)
	switch strings.ToUpper(readDirectStringField(root, "status")) {
	case "COMPLETED":
		return true, readFalQueueError(root)
	case "IN_QUEUE", "IN_PROGRESS", "":
		return false, ""
	default:
		return false, ""
	}
}

// readFalQueueError fal 只在任务失败时给出 error（人话）/ error_type（机器码）。
func readFalQueueError(root map[string]any) string {
	if message := readDirectStringField(root, "error"); message != "" {
		return message
	}
	if kind := readDirectStringField(root, "error_type"); kind != "" {
		return "Fal 任务失败（" + kind + "）"
	}
	return ""
}

var (
	falImageURLKeys = []string{"images", "image", "url", "image_url", "urls", "data", "output", "result"}
	falVideoURLKeys = []string{"video", "videos", "video_url", "url", "output", "data", "result"}
)

// copyFalImageResponse 在画布后端替用户把队列跑完。
//
// 账号渠道（登录后）的图片请求不会走前端的直连适配器，而是打到 /api/v1/images/generations，
// 上游返回的却是"已入队"的 request_id。不在这里跟进队列的话，画布只会拿到一个没有图片的响应。
func copyFalImageResponse(w http.ResponseWriter, response *http.Response, request *http.Request, channel model.ModelChannel, logContext aiLogContext, onFailure func()) bool {
	if !service.IsFalChannel(channel) || !isDirectImageEndpoint(logContext.Endpoint) {
		return false
	}
	payload, _ := io.ReadAll(response.Body)
	// 少数端点直接出图，不经过队列。
	if urls := readDirectMediaURLs(payload, falImageURLKeys); len(urls) > 0 {
		writeDirectImagesResponse(w, response.StatusCode, urls, logContext)
		return true
	}
	submitted := decodeDirectQueuePayload(payload)
	requestID := falRequestIDFromPayload(payload)
	if requestID == "" {
		writeDirectRawResponse(w, response, payload, logContext)
		return true
	}
	imageURLs, message := awaitFalImageResult(request, channel, logContext.Model, requestID, submitted)
	if message != "" {
		if onFailure != nil {
			onFailure()
		}
		writeDirectImageError(w, response.StatusCode, message, logContext)
		return true
	}
	writeDirectImagesResponse(w, response.StatusCode, imageURLs, logContext)
	return true
}

func awaitFalImageResult(request *http.Request, channel model.ModelChannel, modelName string, requestID string, submitted map[string]any) ([]string, string) {
	// 上游自己给的地址最准（嵌套模型 ID 的队列路径由它决定），但只接受同源地址。
	statusTarget := firstNonEmpty(sameHostQueueURL(readDirectStringField(submitted, "status_url"), channel), falQueueActionURL(channel, modelName, requestID, "status"))
	if statusTarget == "" {
		return nil, "Fal 任务缺少查询地址，请检查渠道地址与模型名"
	}
	if _, message := pollDirectQueue(request, channel, statusTarget, readFalQueueProgress); message != "" {
		return nil, message
	}
	resultTarget := firstNonEmpty(sameHostQueueURL(readDirectStringField(submitted, "response_url"), channel), falQueueActionURL(channel, modelName, requestID, "response"))
	if resultTarget == "" {
		return nil, "Fal 任务缺少结果地址，请检查渠道地址与模型名"
	}
	payload, status, err := directQueueGET(request, channel, resultTarget)
	if err != nil {
		return nil, err.Error()
	}
	if status >= http.StatusBadRequest {
		return nil, readUpstreamAIErrorMessage(payload, status)
	}
	imageURLs := readDirectMediaURLs(payload, falImageURLKeys)
	if len(imageURLs) == 0 {
		return nil, "Fal 任务已完成但没有返回图片地址"
	}
	return imageURLs, ""
}

// falVideoResponse 处理视频建任务与轮询两段响应。
func falVideoResponse(payload []byte, request *http.Request, channel model.ModelChannel, _ string, status bool) ([]byte, bool) {
	if !service.IsFalChannel(channel) {
		return nil, false
	}
	if !status {
		return transformFalVideoCreateResponse(payload)
	}
	return transformFalVideoStatusResponse(payload, request, channel)
}

func transformFalVideoCreateResponse(payload []byte) ([]byte, bool) {
	requestID := falRequestIDFromPayload(payload)
	if requestID == "" {
		return nil, false
	}
	return marshalDirectMap(map[string]any{"id": requestID, "task_id": requestID, "status": "processing", "progress": 0})
}

func transformFalVideoStatusResponse(payload []byte, request *http.Request, channel model.ModelChannel) ([]byte, bool) {
	root := decodeDirectQueuePayload(payload)
	switch strings.ToUpper(readDirectStringField(root, "status")) {
	case "COMPLETED":
		if message := readFalQueueError(root); message != "" {
			return marshalDirectMap(map[string]any{"status": "failed", "error": message})
		}
		videoURL, message := fetchFalVideoURL(request, channel)
		if message != "" {
			return marshalDirectMap(map[string]any{"status": "failed", "error": message})
		}
		return marshalDirectMap(map[string]any{"status": "completed", "progress": 100, "video_url": videoURL, "url": videoURL})
	case "IN_QUEUE", "IN_PROGRESS", "":
		return marshalDirectMap(map[string]any{"status": "processing"})
	default:
		return marshalDirectMap(map[string]any{"status": "processing"})
	}
}

// fetchFalVideoURL 状态端点只说"完成了"，产物要去结果端点取。
func fetchFalVideoURL(request *http.Request, channel model.ModelChannel) (string, string) {
	if request == nil || request.URL == nil {
		return "", "Fal 任务缺少查询地址"
	}
	statusTarget := request.URL.String()
	if !strings.HasSuffix(statusTarget, "/status") {
		return "", "Fal 任务查询地址异常，无法获取生成结果"
	}
	payload, status, err := directQueueGET(request, channel, strings.TrimSuffix(statusTarget, "/status")+"/response")
	if err != nil {
		return "", err.Error()
	}
	if status >= http.StatusBadRequest {
		return "", readUpstreamAIErrorMessage(payload, status)
	}
	videoURL := firstNonEmpty(readDirectMediaURLs(payload, falVideoURLKeys)...)
	if videoURL == "" {
		return "", "Fal 视频任务已完成但没有返回视频地址"
	}
	return videoURL, ""
}

func readFalVideoError(payload []byte, channel model.ModelChannel, _ string, status bool) string {
	if !service.IsFalChannel(channel) || !status {
		return ""
	}
	return readFalQueueError(decodeDirectQueuePayload(payload))
}

func decodeDirectBodyObject(raw []byte, label string) (map[string]any, error) {
	body := map[string]any{}
	if len(raw) == 0 {
		return body, nil
	}
	if err := json.Unmarshal(raw, &body); err != nil {
		return nil, errors.New(label + " 渠道只接受 JSON 对象参数")
	}
	return body, nil
}

// applyDirectModelParams 让模型专属参数最后覆盖通用映射，保证用户始终有最终解释权。
func applyDirectModelParams(out map[string]any, params map[string]any) {
	for key, value := range params {
		out[key] = value
	}
}
