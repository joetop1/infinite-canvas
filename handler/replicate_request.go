package handler

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/tigerowo/infinite-canvas/model"
	"github.com/tigerowo/infinite-canvas/service"
)

// Replicate 预测接口的报文转译。
//
// 与 Fal 一样，每个模型的 input schema 各不相同，这里只映射通用字段并把结果包进 input：
//
//	{"input": {"prompt": ..., "image": ...}, "version": "..."}   // 社区模型需要 version
//	{"input": {...}}                                             // 官方模型走 /models/{owner}/{name}/predictions
//
// 模型专属参数同样通过模型名 query 传入，例如
// `black-forest-labs/flux-dev?num_inference_steps=28&guidance=3.5`。
// 模型名支持 Replicate 官方写法 `owner/name:versionhash`，也支持 `?version=hash`。
func normalizeReplicateDirectBody(raw []byte, contentType string, modelName string, endpoint string) ([]byte, error) {
	body, err := decodeDirectRequestBody(raw, contentType, "Replicate")
	if err != nil {
		return nil, err
	}
	spec := parseDirectModelSpec(modelName)
	target, version, err := splitReplicateModel(spec.ModelPath, spec.Version)
	if err != nil {
		return nil, err
	}

	input := map[string]any{}
	if prompt := readDirectString(body["prompt"]); prompt != "" {
		input["prompt"] = prompt
	}

	switch endpoint {
	case "/images/generations":
		if count, ok := readDirectCount(body["n"]); ok {
			input["num_outputs"] = count
		}
	case "/images/edits":
		images := readDirectReferences(body, "image")
		if len(images) == 0 {
			return nil, errors.New("Replicate 图片编辑需要至少一张参考图")
		}
		applyDirectMediaField(input, spec, images, "image", "image_input", directImageFieldKey, directImagePluralKey)
		if count, ok := readDirectCount(body["n"]); ok {
			input["num_outputs"] = count
		}
	case "/videos":
		images := readDirectReferences(body, "input_reference[]")
		if len(images) == 0 {
			images = readDirectReferences(body, "first_frame_url")
		}
		applyDirectMediaField(input, spec, images, "image", "image_input", directImageFieldKey, directImagePluralKey)
		videos := readDirectReferences(body, "video_reference[]")
		applyDirectMediaField(input, spec, videos, "video", "video_input", directVideoFieldKey, directVideoPluralKey)
		if len(readDirectReferences(body, "audio_reference[]")) > 0 {
			return nil, errors.New("Replicate 渠道暂不支持参考音频，请改用参考图片或参考视频")
		}
		if ratio := directAspectRatio(body["size"]); ratio != "" {
			input["aspect_ratio"] = ratio
		}
		// Replicate 会按 schema 严格校验类型，时长必须是数字。
		if duration := readDirectString(body["seconds"]); duration != "" {
			if number, err := strconv.Atoi(duration); err == nil {
				input["duration"] = number
			} else {
				input["duration"] = duration
			}
		}
	default:
		return nil, errors.New("Replicate 渠道不支持该接口")
	}

	applyDirectModelParams(input, spec.Params)
	out := map[string]any{"input": input}
	if version != "" {
		out["version"] = version
	}
	if target == "" && version == "" {
		return nil, errors.New("缺少 Replicate 模型标识，请在模型名中填写如 owner/name 或 owner/name:versionhash")
	}
	return json.Marshal(out)
}

func prepareReplicateRequest(input aiProtocolRequest) (aiProtocolRequest, bool, error) {
	if !service.IsReplicateChannel(input.channel) || !isReplicateEndpoint(input.endpoint) {
		return input, false, nil
	}
	input.failureLabel = "Replicate"
	body, err := normalizeReplicateDirectBody(input.body, input.contentType, input.modelName, input.endpoint)
	if err != nil {
		return input, true, err
	}
	input.body = body
	input.contentType = "application/json"
	return input, true, nil
}

// isReplicateEndpoint 与 Fal 同理：三种 mode 共用一份转译结果，只认领能转译的接口。
func isReplicateEndpoint(endpoint string) bool {
	switch endpoint {
	case "/images/generations", "/images/edits", "/videos":
		return true
	default:
		return false
	}
}

// replicateUpstreamPath：提交时社区模型需要自带版本号，走 /predictions；
// 官方模型走 /models/{owner}/{name}/predictions；轮询统一走 /predictions/{id}。
func replicateUpstreamPath(modelName string, path string) (string, bool) {
	if isReplicateEndpoint(path) {
		spec := parseDirectModelSpec(modelName)
		target, version, err := splitReplicateModel(spec.ModelPath, spec.Version)
		if err != nil {
			return path, true
		}
		if version != "" {
			return "/predictions", true
		}
		return "/models/" + target + "/predictions", true
	}
	if taskID, ok := directTaskIDFromPath(path); ok {
		return "/predictions/" + url.PathEscape(taskID), true
	}
	return path, true
}

func replicatePredictionURL(channel model.ModelChannel, predictionID string) string {
	if strings.TrimSpace(predictionID) == "" {
		return ""
	}
	return service.BuildModelChannelURL(channel, "/predictions/"+url.PathEscape(predictionID))
}

// readReplicateGetURL 建任务响应里自带查询地址（urls.get），同源时优先用它。
func readReplicateGetURL(root map[string]any) string {
	urls, ok := root["urls"].(map[string]any)
	if !ok {
		return ""
	}
	return readDirectStringField(urls, "get")
}

// replicateMediaURLKeys 故意不含 "urls"：Replicate 轮询报文里的 urls.get / urls.cancel
// 是接口地址而不是产物地址，收进来会把接口地址当成生成的图片。
var replicateMediaURLKeys = []string{"output", "video", "videos", "video_url", "image", "images", "url", "data"}

// readReplicateProgress 解读轮询响应。
func readReplicateProgress(payload []byte) (bool, string) {
	root := decodeDirectQueuePayload(payload)
	switch strings.ToLower(readDirectStringField(root, "status")) {
	case "succeeded":
		return true, ""
	case "failed", "canceled", "cancelled", "aborted":
		return false, readReplicateTaskError(root)
	default:
		return false, ""
	}
}

func readReplicateTaskError(root map[string]any) string {
	if message := readDirectStringField(root, "error"); message != "" {
		return message
	}
	if strings.EqualFold(readDirectStringField(root, "status"), "aborted") {
		return "Replicate 任务已中止"
	}
	return "Replicate 任务失败"
}

// copyReplicateImageResponse 与 Fal 同理：账号渠道下由后端替用户把预测跑完，
// 再把产物包成 OpenAI 图片响应，画布侧感知不到异步过程。
func copyReplicateImageResponse(w http.ResponseWriter, response *http.Response, request *http.Request, channel model.ModelChannel, logContext aiLogContext, onFailure func()) bool {
	if !service.IsReplicateChannel(channel) || !isDirectImageEndpoint(logContext.Endpoint) {
		return false
	}
	payload, _ := io.ReadAll(response.Body)
	if urls := readDirectMediaURLs(payload, replicateMediaURLKeys); len(urls) > 0 {
		writeDirectImagesResponse(w, response.StatusCode, urls, logContext)
		return true
	}
	root := decodeDirectQueuePayload(payload)
	predictionID := readDirectStringField(root, "id")
	if predictionID == "" {
		writeDirectRawResponse(w, response, payload, logContext)
		return true
	}
	target := firstNonEmpty(sameHostQueueURL(readReplicateGetURL(root), channel), replicatePredictionURL(channel, predictionID))
	if target == "" {
		if onFailure != nil {
			onFailure()
		}
		writeDirectImageError(w, response.StatusCode, "Replicate 任务缺少查询地址，请检查渠道地址", logContext)
		return true
	}
	result, message := pollDirectQueue(request, channel, target, readReplicateProgress)
	if message != "" {
		if onFailure != nil {
			onFailure()
		}
		writeDirectImageError(w, response.StatusCode, message, logContext)
		return true
	}
	imageURLs := readDirectMediaURLs(result, replicateMediaURLKeys)
	if len(imageURLs) == 0 {
		if onFailure != nil {
			onFailure()
		}
		writeDirectImageError(w, response.StatusCode, "Replicate 任务已完成但没有返回图片地址", logContext)
		return true
	}
	writeDirectImagesResponse(w, response.StatusCode, imageURLs, logContext)
	return true
}

// replicateVideoResponse 处理视频建任务与轮询两段响应。
func replicateVideoResponse(payload []byte, _ *http.Request, channel model.ModelChannel, _ string, status bool) ([]byte, bool) {
	if !service.IsReplicateChannel(channel) {
		return nil, false
	}
	root := decodeDirectQueuePayload(payload)
	predictionID := readDirectStringField(root, "id")
	if predictionID == "" {
		return nil, false
	}
	if !status {
		return marshalDirectMap(map[string]any{"id": predictionID, "task_id": predictionID, "status": "processing", "progress": 0})
	}
	switch strings.ToLower(readDirectStringField(root, "status")) {
	case "succeeded":
		videoURL := firstNonEmpty(readDirectMediaURLs(payload, replicateMediaURLKeys)...)
		if videoURL == "" {
			return marshalDirectMap(map[string]any{"status": "failed", "error": "Replicate 视频任务已完成但没有返回视频地址"})
		}
		return marshalDirectMap(map[string]any{"status": "completed", "progress": 100, "video_url": videoURL, "url": videoURL})
	case "failed", "canceled", "cancelled", "aborted":
		return marshalDirectMap(map[string]any{"status": "failed", "error": readReplicateTaskError(root)})
	default:
		return marshalDirectMap(map[string]any{"status": "processing"})
	}
}

func readReplicateVideoError(payload []byte, channel model.ModelChannel, _ string, status bool) string {
	if !service.IsReplicateChannel(channel) || !status {
		return ""
	}
	root := decodeDirectQueuePayload(payload)
	switch strings.ToLower(readDirectStringField(root, "status")) {
	case "failed", "canceled", "cancelled", "aborted":
		return readReplicateTaskError(root)
	default:
		return ""
	}
}

// splitReplicateModel 解析 `owner/name`、`owner/name:versionhash`，返回模型路径与版本号。
func splitReplicateModel(modelPath string, version string) (string, string, error) {
	target := strings.Trim(strings.TrimSpace(modelPath), "/")
	if target == "" {
		if strings.TrimSpace(version) == "" {
			return "", "", errors.New("缺少 Replicate 模型标识")
		}
		return "", strings.TrimSpace(version), nil
	}
	if index := strings.LastIndex(target, ":"); index > 0 && !strings.Contains(target[index+1:], "/") {
		if strings.TrimSpace(version) == "" {
			version = strings.TrimSpace(target[index+1:])
		}
		target = target[:index]
	}
	parts := strings.Split(target, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", strings.TrimSpace(version), errors.New("Replicate 模型名需为 owner/name 或 owner/name:version")
	}
	return target, strings.TrimSpace(version), nil
}
