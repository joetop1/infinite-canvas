package handler

import (
	"encoding/json"
	"errors"
	"net/url"
	"strconv"
	"strings"

	"github.com/tigerowo/infinite-canvas/model"
)

// OpenRouter 的视频接口使用 JSON，不能直接转发 OpenAI 的 multipart 请求。
func isOpenRouterChannel(channel model.ModelChannel) bool {
	base, err := url.Parse(strings.TrimSpace(channel.BaseURL))
	return err == nil && strings.EqualFold(base.Hostname(), "openrouter.ai") && (channel.Protocol == "" || strings.EqualFold(channel.Protocol, "openai"))
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
	input.body, err = json.Marshal(out)
	input.contentType = "application/json"
	return input, true, err
}
