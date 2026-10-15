package handler

import (
	"encoding/json"
	"errors"
	"strconv"
	"strings"
)

// H3 Max distinguishes reference lists from first/last-frame inputs.
func normalizeFalH3MaxBody(body map[string]any, spec directModelSpec, out map[string]any) ([]byte, error) {
	images := readDirectReferences(body, "input_reference[]")
	videos := readDirectReferences(body, "video_reference[]")
	audios := readDirectReferences(body, "audio_reference[]")
	if len(images)+len(videos)+len(audios) == 0 {
		return nil, errors.New("MiniMax H3 Max 全能参考需要至少一份参考图片、视频或音频；首尾帧不能代替参考素材")
	}
	if len(images) > 9 || len(videos) > 3 || len(audios) > 3 || len(images)+len(videos)+len(audios) > 12 {
		return nil, errors.New("MiniMax H3 Max 最多 9 张参考图、3 段参考视频、3 段参考音频，总计不超过 12 份")
	}
	for field, values := range map[string][]string{"reference_image_urls": images, "reference_video_urls": videos, "reference_audio_urls": audios} {
		if len(values) > 0 {
			out[field] = values
		}
	}
	if first := readDirectString(body["first_frame_url"]); first != "" {
		out["image_url"] = first
	}
	if last := readDirectString(body["last_frame_url"]); last != "" {
		out["end_image_url"] = last
	}
	duration := 5.0
	if value := readDirectString(body["seconds"]); value != "" {
		var err error
		duration, err = strconv.ParseFloat(value, 64)
		if err != nil || duration < 0.92 || duration > 15 {
			return nil, errors.New("MiniMax H3 Max 视频时长需为 0.92–15 秒")
		}
	}
	out["duration"] = duration
	resolution := strings.ToUpper(readDirectString(body["resolution_name"]))
	if resolution == "" || resolution == "720" || resolution == "720P" {
		resolution = "768P"
	}
	if !strings.HasSuffix(resolution, "P") {
		resolution += "P"
	}
	if resolution != "480P" && resolution != "768P" && resolution != "1080P" {
		return nil, errors.New("MiniMax H3 Max 只支持 480p、768p 或 1080p，请重新选择清晰度")
	}
	out["resolution"] = resolution
	out["prompt_expansion_mode"] = "balanced"
	if ratio := directAspectRatio(body["size"]); ratio != "" {
		out["aspect_ratio"] = ratio
	}
	applyDirectModelParams(out, spec.Params)
	return json.Marshal(out)
}
