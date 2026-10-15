package service

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/santhosh-tekuri/jsonschema/v5"
	"github.com/tigerowo/infinite-canvas/model"
)

// PrepareMediaContractInput is used only by channels explicitly opting into strict contracts.
func PrepareMediaContractInput(channel model.ModelChannel, name, endpoint string, body map[string]any, options map[string]any) (map[string]any, error) {
	if !channel.StrictContracts {
		return nil, fmt.Errorf("此渠道尚未启用统一模型适配")
	}
	entry := loadMediaContract(channel, name)
	if entry.Description.Status != "ready" {
		return nil, fmt.Errorf("模型尚未适配：%s", entry.Description.Reason)
	}
	if endpoint == "/videos" && entry.Description.Capability != "video" || strings.HasPrefix(endpoint, "/images/") && entry.Description.Capability != "image" {
		return nil, fmt.Errorf("模型类型与当前生成任务不匹配")
	}
	out := map[string]any{}
	for key, raw := range entry.Properties {
		if schema := effectiveContractSchema(raw, 0); schema != nil && schema.Default != nil {
			out[key] = schema.Default
		}
	}
	bindings := entry.Description.Bindings
	assign := func(role string, value any) error {
		field, ok := bindings[role]
		if !ok {
			return fmt.Errorf("此模型没有适配%s参数", mediaRoleLabel(role))
		}
		converted, err := castContractValue(value, field)
		if err != nil {
			return fmt.Errorf("%s：%w", mediaRoleLabel(role), err)
		}
		out[field.Name] = converted
		return nil
	}
	if err := assign("prompt", contractText(body["prompt"])); err != nil {
		return nil, err
	}
	images := contractReferences(body["input_reference[]"])
	if endpoint == "/images/edits" {
		images = contractReferences(body["image"])
	}
	videos, audios := contractReferences(body["video_reference[]"]), contractReferences(body["audio_reference[]"])
	count := len(images) + len(videos) + len(audios)
	if entry.Description.RequiresReference && count == 0 && (strings.Contains(name, "reference-to-video") || contractText(body["first_frame_url"]) == "") || endpoint == "/images/edits" && len(images) == 0 {
		return nil, fmt.Errorf("此模型需要参考素材，请添加已支持的图片、视频或音频")
	}
	if entry.Description.MaxReferences > 0 && count > entry.Description.MaxReferences {
		return nil, fmt.Errorf("此模型参考素材总数不能超过 %d", entry.Description.MaxReferences)
	}
	for _, item := range []struct {
		role   string
		values []string
	}{{"images", images}, {"videos", videos}, {"audios", audios}} {
		if len(item.values) == 0 {
			continue
		}
		field, ok := bindings[item.role]
		if !ok {
			return nil, fmt.Errorf("此模型不支持%s", mediaRoleLabel(item.role))
		}
		if field.MaxItems > 0 && len(item.values) > field.MaxItems {
			return nil, fmt.Errorf("%s最多支持 %d 份", mediaRoleLabel(item.role), field.MaxItems)
		}
		if field.Type == "array" {
			out[field.Name] = item.values
		} else if field.Type == "string" && len(item.values) == 1 {
			out[field.Name] = item.values[0]
		} else {
			return nil, fmt.Errorf("%s的输入结构尚未适配", mediaRoleLabel(item.role))
		}
	}
	for _, item := range []struct{ key, role string }{{"first_frame_url", "first_frame"}, {"last_frame_url", "last_frame"}} {
		value := contractText(body[item.key])
		if value == "" {
			continue
		}
		if item.role == "first_frame" && bindings[item.role].Name == "" {
			if len(images) > 0 {
				return nil, fmt.Errorf("此模型的首帧与普通参考图片不能同时提交")
			}
			if err := assign("images", value); err != nil {
				return nil, err
			}
		} else if err := assign(item.role, value); err != nil {
			return nil, err
		}
	}
	for _, item := range []struct{ key, role string }{{"seconds", "duration"}, {"resolution_name", "resolution"}, {"negative_prompt", "negative_prompt"}, {"quality", "quality"}, {"keep_audio", "keep_audio"}, {"video_generate_audio", "audio"}, {"n", "count"}} {
		value, exists := body[item.key]
		if !exists || contractText(value) == "" {
			continue
		}
		if _, ok := bindings[item.role]; !ok {
			if item.role == "audio" && contractText(value) == "true" && !entry.Description.NativeAudio {
				return nil, fmt.Errorf("此模型没有已适配的音频生成功能")
			}
			continue
		}
		if err := assign(item.role, value); err != nil {
			return nil, err
		}
	}
	if size := contractText(body["size"]); size != "" && size != "auto" && size != "adaptive" {
		if _, ok := bindings["ratio"]; ok {
			if err := assign("ratio", contractAspectRatio(size)); err != nil {
				return nil, err
			}
		} else if _, ok := bindings["size"]; ok {
			if err := assign("size", size); err != nil {
				return nil, err
			}
		} else if bindings["width"].Name != "" && bindings["height"].Name != "" {
			parts := strings.Split(strings.ToLower(size), "x")
			if len(parts) != 2 {
				return nil, fmt.Errorf("此模型需要像素尺寸，请选择宽度和高度")
			}
			if err := assign("width", parts[0]); err != nil {
				return nil, err
			}
			if err := assign("height", parts[1]); err != nil {
				return nil, err
			}
		}
	}
	for key, value := range options {
		schema := effectiveContractSchema(entry.Properties[key], 0)
		if schema == nil {
			return nil, fmt.Errorf("模型专属参数 %s 不在官方规范中", key)
		}
		converted, err := castContractValue(value, contractField(key, schema))
		if err != nil {
			return nil, fmt.Errorf("模型专属参数 %s：%w", key, err)
		}
		out[key] = converted
	}
	encoded, err := json.Marshal(out)
	if err != nil {
		return nil, fmt.Errorf("模型参数无法序列化")
	}
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.UseNumber()
	if err := decoder.Decode(&out); err != nil {
		return nil, err
	}
	finalCount := 0
	for _, role := range []string{"images", "videos", "audios"} {
		if field, ok := bindings[role]; ok {
			finalCount += len(contractReferences(out[field.Name]))
		}
	}
	if entry.Description.MaxReferences > 0 && finalCount > entry.Description.MaxReferences {
		return nil, fmt.Errorf("实际提交的参考素材总数超过模型限制")
	}
	if entry.Description.RequiresReference && strings.Contains(name, "reference-to-video") && finalCount == 0 {
		return nil, fmt.Errorf("实际提交的请求中没有参考素材，尚未调用模型")
	}
	if err := entry.Schema.Validate(out); err != nil {
		return nil, fmt.Errorf("模型参数不符合官方规范：%s", contractValidationMessage(err))
	}
	return out, nil
}

func contractField(name string, schema *jsonschema.Schema) model.MediaContractField {
	field := model.MediaContractField{Name: name, Default: schema.Default, Values: schema.Enum}
	for _, kind := range schema.Types {
		if kind != "null" {
			field.Type = kind
			break
		}
	}
	return field
}

func castContractValue(value any, field model.MediaContractField) (any, error) {
	if len(field.Values) > 0 {
		for _, allowed := range field.Values {
			if strings.EqualFold(contractText(value), contractText(allowed)) {
				return allowed, nil
			}
			if strings.EqualFold(strings.TrimSuffix(strings.ToLower(contractText(value)), "p"), strings.TrimSuffix(strings.ToLower(contractText(allowed)), "p")) && strings.Contains(strings.ToLower(field.Name), "resolution") {
				return allowed, nil
			}
		}
		return nil, fmt.Errorf("不支持当前选项 %s，请按模型能力重新选择", contractText(value))
	}
	switch field.Type {
	case "string":
		return contractText(value), nil
	case "integer", "number":
		number, err := strconv.ParseFloat(contractText(value), 64)
		if err != nil || math.IsNaN(number) || math.IsInf(number, 0) || field.Type == "integer" && number != math.Trunc(number) {
			return nil, fmt.Errorf("必须填写有效的%s", field.Type)
		}
		return number, nil
	case "boolean":
		result, err := strconv.ParseBool(contractText(value))
		if err != nil {
			return nil, fmt.Errorf("必须为 true 或 false")
		}
		return result, nil
	case "array":
		if values, ok := value.([]string); ok {
			return values, nil
		}
		if values, ok := value.([]any); ok {
			return values, nil
		}
		return []string{contractText(value)}, nil
	case "object":
		if _, ok := value.(map[string]any); ok {
			return value, nil
		}
		parts := strings.Split(strings.ToLower(contractText(value)), "x")
		if len(parts) == 2 {
			w, ew := strconv.Atoi(parts[0])
			h, eh := strconv.Atoi(parts[1])
			if ew == nil && eh == nil && w > 0 && h > 0 {
				return map[string]any{"width": w, "height": h}, nil
			}
		}
		return nil, fmt.Errorf("结构化尺寸尚未适配，请选择像素尺寸")
	}
	return nil, fmt.Errorf("此参数的结构尚未适配")
}

func contractReferences(value any) []string {
	result := []string{}
	switch value := value.(type) {
	case string:
		if value != "" {
			result = append(result, value)
		}
	case []any:
		for _, item := range value {
			if text := contractText(item); text != "" {
				result = append(result, text)
			}
		}
	case []string:
		result = append(result, value...)
	}
	return result
}

func contractText(value any) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(value))
}

func contractAspectRatio(size string) string {
	if strings.Contains(size, ":") {
		return size
	}
	parts := strings.Split(strings.ToLower(size), "x")
	if len(parts) != 2 {
		return size
	}
	w, _ := strconv.Atoi(parts[0])
	h, _ := strconv.Atoi(parts[1])
	if w <= 0 || h <= 0 {
		return size
	}
	a, b := w, h
	for b != 0 {
		a, b = b, a%b
	}
	return fmt.Sprintf("%d:%d", w/a, h/a)
}

func mediaRoleLabel(role string) string {
	labels := map[string]string{"images": "参考图片", "videos": "参考视频", "audios": "参考音频", "first_frame": "首帧", "last_frame": "尾帧", "duration": "时长", "resolution": "清晰度", "ratio": "比例", "size": "尺寸", "prompt": "提示词", "audio": "音频生成", "count": "数量", "quality": "质量"}
	if label := labels[role]; label != "" {
		return label
	}
	return role
}

func contractValidationMessage(err error) string {
	if item, ok := err.(*jsonschema.ValidationError); ok {
		for len(item.Causes) > 0 {
			item = item.Causes[0]
		}
		return strings.TrimPrefix(item.InstanceLocation, "/") + " " + item.Message
	}
	return "输入结构校验失败"
}
