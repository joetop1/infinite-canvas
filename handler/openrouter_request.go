package handler

import (
	"encoding/json"
	"errors"
	"strconv"
	"strings"

	"github.com/tigerowo/infinite-canvas/model"
	"github.com/tigerowo/infinite-canvas/service"
)

// OpenRouter 的视频接口使用 JSON，不能直接转发 OpenAI 的 multipart 请求。
func isOpenRouterChannel(channel model.ModelChannel) bool {
	return service.IsOpenRouterChannel(channel)
}

func prepareOpenRouterVideoRequest(input aiProtocolRequest) (aiProtocolRequest, bool, error) {
	if !isOpenRouterChannel(input.channel) || input.endpoint != "/videos" {
		return input, false, nil
	}
	input.failureLabel = "OpenRouter"
	body, err := decodeDirectRequestBody(input.body, input.contentType, "OpenRouter")
	if err != nil {
		return input, true, err
	}
	out := map[string]any{"model": input.modelName}
	for _, key := range []string{"prompt", "aspect_ratio", "size", "resolution", "duration", "frame_images", "input_references", "generate_audio", "seed", "provider", "callback_url"} {
		if value, ok := body[key]; ok {
			out[key] = value
		}
	}
	if duration := firstNonEmpty(readDirectString(body["duration"]), readDirectString(body["seconds"])); duration != "" {
		number, err := strconv.Atoi(duration)
		if err != nil || number < 1 {
			return input, true, errors.New("OpenRouter 视频时长必须是正整数秒")
		}
		out["duration"] = number
	}
	if resolution := firstNonEmpty(readDirectString(body["resolution"]), readDirectString(body["resolution_name"])); resolution != "" {
		out["resolution"] = resolution
	}
	audio, hasAudio := body["generate_audio"]
	if !hasAudio {
		audio, hasAudio = body["video_generate_audio"]
	}
	if hasAudio {
		if value, ok := audio.(bool); ok {
			out["generate_audio"] = value
		} else {
			value, err := strconv.ParseBool(readDirectString(audio))
			if err != nil {
				return input, true, errors.New("OpenRouter 音频开关必须为 true 或 false")
			}
			out["generate_audio"] = value
		}
	}
	refs := []map[string]any{}
	for _, field := range []struct{ key, kind string }{{"input_reference[]", "image"}, {"video_reference[]", "video"}, {"audio_reference[]", "audio"}} {
		for _, reference := range readDirectReferences(body, field.key) {
			kind := field.kind + "_url"
			refs = append(refs, map[string]any{"type": kind, kind: map[string]any{"url": reference}})
		}
	}
	if len(refs) > 0 {
		out["input_references"] = refs
	}
	frames := []map[string]any{}
	for _, field := range []struct{ key, kind string }{{"first_frame_url", "first_frame"}, {"last_frame_url", "last_frame"}} {
		for _, reference := range readDirectReferences(body, field.key) {
			frames = append(frames, map[string]any{"type": "image_url", "image_url": map[string]any{"url": reference}, "frame_type": field.kind})
		}
	}
	if len(frames) > 0 {
		out["frame_images"] = frames
	}
	if strings.EqualFold(input.channel.Protocol, "openrouter") {
		metadata := input.channel.ModelMetadata[input.modelName]
		if len(metadata) == 0 {
			catalog, fetchErr := service.FetchOpenRouterCatalog(input.channel)
			if fetchErr != nil { return input, true, fetchErr }
			metadata = catalog.ModelMetadata[input.modelName]
		}
		var caps struct { Audio *bool `json:"generate_audio"`; Durations []int `json:"supported_durations"`; Resolutions []string `json:"supported_resolutions"`; Ratios []string `json:"supported_aspect_ratios"`; Frames []string `json:"supported_frame_images"` }
		if len(metadata) == 0 || json.Unmarshal(metadata, &caps) != nil { return input, true, errors.New("OpenRouter 模型未在目录中，请重新拉取模型") }
		encodedRefs, _ := json.Marshal(out["input_references"])
		var mediaRefs []struct { Type string `json:"type"` }
		_ = json.Unmarshal(encodedRefs, &mediaRefs)
		omni := service.OpenRouterOmniModel(input.modelName)
		for _, reference := range mediaRefs { if (reference.Type == "video_url" || reference.Type == "audio_url") && !omni { return input, true, errors.New("此 OpenRouter 模型尚未确认支持视频／音频参考，已阻止提交") } }
		encodedFrames, _ := json.Marshal(out["frame_images"])
		var frameRefs []struct { Type string `json:"frame_type"` }
		_ = json.Unmarshal(encodedFrames, &frameRefs)
		if len(frameRefs) > 0 && len(mediaRefs) > 0 { return input, true, errors.New("首尾帧与普通参考素材不能同时提交") }
		for _, frame := range frameRefs {
			valid := false
			for _, allowed := range caps.Frames { if allowed == frame.Type { valid = true } }
			if !valid { return input, true, errors.New("此 OpenRouter 模型不支持所选首尾帧") }
		}
		if audio, ok := out["generate_audio"].(bool); ok && audio && (caps.Audio == nil || !*caps.Audio) { return input, true, errors.New("此 OpenRouter 模型未确认支持生成音频开关") }
		if len(caps.Durations) > 0 {
			duration := readDirectString(out["duration"])
			valid := false
			for _, value := range caps.Durations { if duration == strconv.Itoa(value) { valid = true } }
			if duration != "" && !valid { return input, true, errors.New("此 OpenRouter 模型不支持所选时长") }
		}
		for _, choice := range []struct { key string; values []string }{{"resolution", caps.Resolutions}, {"aspect_ratio", caps.Ratios}} {
			value := readDirectString(out[choice.key])
			if value == "" || len(choice.values) == 0 { continue }
			valid := false
			for _, allowed := range choice.values { if value == allowed { valid = true } }
			if !valid { return input, true, errors.New("此 OpenRouter 模型不支持所选分辨率或比例") }
		}
	}
	input.body, err = json.Marshal(out)
	input.contentType = "application/json"
	return input, true, err
}

func prepareOpenRouterImageRequest(input aiProtocolRequest) (aiProtocolRequest, bool, error) {
	if !isOpenRouterChannel(input.channel) || (input.endpoint != "/images/generations" && input.endpoint != "/images/edits") { return input, false, nil }
	body, err := decodeDirectRequestBody(input.body, input.contentType, "OpenRouter")
	if err != nil { return input, true, err }
	out := map[string]any{"model": input.modelName}
	for _, key := range []string{"prompt", "n", "aspect_ratio", "resolution", "quality", "output_format", "background", "output_compression", "seed", "input_references", "provider"} {
		if value, found := body[key]; found { out[key] = value }
	}
	if _, found := out["aspect_ratio"]; !found { if ratio := directAspectRatio(body["size"]); ratio != "" { out["aspect_ratio"] = ratio } }
	refs := []map[string]any{}
	for _, key := range []string{"image", "image[]", "input_reference[]"} {
		for _, reference := range readDirectReferences(body, key) { refs = append(refs, map[string]any{"type": "image_url", "image_url": map[string]any{"url": reference}}) }
	}
	if len(refs) > 0 { out["input_references"] = refs }
	input.body, err = json.Marshal(out)
	input.path, input.contentType = "/images", "application/json"
	return input, true, err
}

// Only actual completed outputs are video URLs; polling URLs must never finish a task.
func transformOpenRouterVideoPayload(payload []byte, channel model.ModelChannel) ([]byte, bool) {
	if !isOpenRouterChannel(channel) { return nil, false }
	var root map[string]any
	if json.Unmarshal(payload, &root) != nil { return nil, false }
	if readDirectString(root["id"]) == "" { return nil, false }
	out := map[string]any{"object": "video"}
	for _, key := range []string{"id", "status", "progress", "model", "duration", "size", "error", "created_at", "completed_at"} {
		if value, found := root[key]; found { out[key] = value }
	}
	if service.IsCompletedVideoTaskStatus(readDirectString(root["status"])) {
		for _, key := range []string{"unsigned_urls", "urls"} {
			if list, ok := root[key].([]any); ok && len(list) > 0 { out["url"] = list[0]; break }
		}
		if _, found := out["url"]; !found { if value, found := root["url"]; found { out["url"] = value } }
	}
	body, err := json.Marshal(out)
	return body, err == nil
}
