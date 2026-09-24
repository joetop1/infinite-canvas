package handler

import (
	"encoding/json"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

var (
	aspectRatioPattern = regexp.MustCompile(`^(\d+)\s*:\s*(\d+)$`)
	pixelSizePattern   = regexp.MustCompile(`^(\d+)x(\d+)$`)
)

// 第三方平台（Fal / Replicate）的模型标识支持用 query 追加模型专属参数，
// 例如 `fal-ai/flux/dev?image_size=landscape_16_9&num_inference_steps=28`、
// `black-forest-labs/flux-kontext-pro?input_image_field=image`。
// 这些平台每个模型的输入 schema 都不同，无法在后端穷举，因此统一走"模型名带参数"这条通道。
const (
	directParamsKey      = "params"
	directVersionKey     = "version"
	directImageFieldKey  = "image_field"
	directImagePluralKey = "image_field_plural"
	directVideoFieldKey  = "video_field"
	directVideoPluralKey = "video_field_plural"
)

type directModelSpec struct {
	ModelPath string            // 已剥离 query 的模型标识
	Params    map[string]any    // query 中的模型专属参数，会合并进上游报文
	Fields    map[string]string // 字段名覆写（image_field 等），只用于本地映射，不发给上游
	Version   string            // Replicate 专用：模型版本号
}

// parseDirectModelSpec 拆出模型路径、模型专属参数与字段名覆写。
func parseDirectModelSpec(modelName string) directModelSpec {
	spec := directModelSpec{Params: map[string]any{}, Fields: map[string]string{}}
	raw := strings.TrimSpace(modelName)
	if raw == "" {
		return spec
	}
	parsed, err := url.Parse("https://direct-model.invalid/" + strings.TrimPrefix(raw, "/"))
	if err != nil {
		spec.ModelPath = raw
		return spec
	}
	spec.ModelPath = strings.TrimPrefix(parsed.Path, "/")

	query := parsed.Query()
	for key, values := range query {
		if len(values) == 0 {
			continue
		}
		switch key {
		case directVersionKey:
			spec.Version = strings.TrimSpace(values[0])
		case directImageFieldKey, directImagePluralKey, directVideoFieldKey, directVideoPluralKey:
			if value := strings.TrimSpace(values[0]); value != "" {
				spec.Fields[key] = value
			}
		case directParamsKey:
			mergeDirectJSONParams(spec.Params, values[0])
		default:
			spec.Params[key] = directParamValue(values[0])
		}
	}
	return spec
}

// mergeDirectJSONParams 支持 `?params={"num_inference_steps":28}` 这种需要精确类型的写法。
func mergeDirectJSONParams(target map[string]any, raw string) {
	var values map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(raw)), &values); err != nil {
		return
	}
	for key, value := range values {
		target[key] = value
	}
}

// directParamValue 在无法写 JSON 时做类型推断：布尔、整数、小数、其余按字符串。
func directParamValue(text string) any {
	value := strings.TrimSpace(text)
	switch strings.ToLower(value) {
	case "true":
		return true
	case "false":
		return false
	}
	if number, err := strconv.Atoi(value); err == nil {
		return number
	}
	if number, err := strconv.ParseFloat(value, 64); err == nil {
		return number
	}
	return value
}

func directModelFieldOverride(spec directModelSpec, key string) string {
	return strings.TrimSpace(spec.Fields[key])
}

func readDirectString(value any) string {
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	case json.Number:
		return typed.String()
	case float64:
		return strconv.FormatFloat(typed, 'f', -1, 64)
	case int:
		return strconv.Itoa(typed)
	default:
		return ""
	}
}

// readDirectReferences 收集参考素材占位符；兼容单值与数组两种写法。
func readDirectReferences(body map[string]any, key string) []string {
	value, ok := body[key]
	if !ok {
		return nil
	}
	collect := func(item any) []string {
		text := readDirectString(item)
		if text == "" || directAIReferenceKind(text) == "" {
			return nil
		}
		return []string{text}
	}
	if list, ok := value.([]any); ok {
		result := make([]string, 0, len(list))
		for _, item := range list {
			result = append(result, collect(item)...)
		}
		return result
	}
	return collect(value)
}

func readDirectCount(value any) (int, bool) {
	if value == nil {
		return 0, false
	}
	number, err := strconv.Atoi(readDirectString(value))
	if err != nil || number <= 1 {
		return 0, false
	}
	return number, true
}

// applyDirectMediaField 把参考素材写入平台约定的字段；字段名可用 query 覆盖，
// 因为同一平台上不同模型的参考图字段名并不统一（如 image / image_url / input_image / image_input）。
func applyDirectMediaField(out map[string]any, spec directModelSpec, values []string, singular string, plural string, singularKey string, pluralKey string) {
	if len(values) == 0 {
		return
	}
	if override := directModelFieldOverride(spec, singularKey); override != "" {
		singular = override
	}
	if override := directModelFieldOverride(spec, pluralKey); override != "" {
		plural = override
	}
	if len(values) == 1 {
		out[singular] = values[0]
		return
	}
	out[plural] = values
}

// directAspectRatio 把画布的像素尺寸折算成平台常用的宽高比；已是宽高比时原样返回。
func directAspectRatio(value any) string {
	text := strings.ToLower(readDirectString(value))
	if text == "" {
		return ""
	}
	if matched := aspectRatioPattern.FindStringSubmatch(text); matched != nil {
		return matched[1] + ":" + matched[2]
	}
	matched := pixelSizePattern.FindStringSubmatch(text)
	if matched == nil {
		return ""
	}
	width, _ := strconv.Atoi(matched[1])
	height, _ := strconv.Atoi(matched[2])
	if width <= 0 || height <= 0 {
		return ""
	}
	divisor := greatestCommonDivisor(width, height)
	return strconv.Itoa(width/divisor) + ":" + strconv.Itoa(height/divisor)
}

func greatestCommonDivisor(a int, b int) int {
	for b != 0 {
		a, b = b, a%b
	}
	if a < 0 {
		return -a
	}
	return a
}
