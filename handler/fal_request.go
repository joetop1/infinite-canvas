package handler

import (
	"encoding/json"
	"errors"

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
func normalizeFalDirectBody(raw []byte, modelName string, endpoint string) ([]byte, error) {
	body, err := decodeDirectBodyObject(raw, "Fal")
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
	if input.mode != aiProtocolDirectRequest || !service.IsFalChannel(input.channel) {
		return input, false, nil
	}
	input.failureLabel = "Fal"
	body, err := normalizeFalDirectBody(input.body, input.modelName, input.endpoint)
	if err != nil {
		return input, true, err
	}
	input.body = body
	input.contentType = "application/json"
	return input, true, nil
}

// falUpstreamPath 用模型路径作为提交地址：POST {base}/{modelId}。
func falUpstreamPath(modelName string, path string) (string, bool) {
	switch path {
	case "/images/generations", "/images/edits", "/videos":
		modelID := parseDirectModelSpec(modelName).ModelPath
		if modelID == "" {
			return path, true
		}
		return "/" + modelID, true
	default:
		return path, true
	}
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
