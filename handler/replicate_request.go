package handler

import (
	"encoding/json"
	"errors"
	"strconv"
	"strings"

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
func normalizeReplicateDirectBody(raw []byte, modelName string, endpoint string) ([]byte, error) {
	body, err := decodeDirectBodyObject(raw, "Replicate")
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
	if input.mode != aiProtocolDirectRequest || !service.IsReplicateChannel(input.channel) {
		return input, false, nil
	}
	input.failureLabel = "Replicate"
	body, err := normalizeReplicateDirectBody(input.body, input.modelName, input.endpoint)
	if err != nil {
		return input, true, err
	}
	input.body = body
	input.contentType = "application/json"
	return input, true, nil
}

// replicateUpstreamPath：社区模型需要自带版本号，走 /predictions；官方模型走 /models/{owner}/{name}/predictions。
func replicateUpstreamPath(modelName string, path string) (string, bool) {
	switch path {
	case "/images/generations", "/images/edits", "/videos":
		spec := parseDirectModelSpec(modelName)
		target, version, err := splitReplicateModel(spec.ModelPath, spec.Version)
		if err != nil {
			return path, true
		}
		if version != "" {
			return "/predictions", true
		}
		return "/models/" + target + "/predictions", true
	default:
		return path, true
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
