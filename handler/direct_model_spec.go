package handler

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/url"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

var (
	aspectRatioPattern = regexp.MustCompile(`^(\d+)\s*:\s*(\d+)$`)
	pixelSizePattern   = regexp.MustCompile(`^(\d+)x(\d+)$`)
)

const (
	directReferenceRequestLimit = 64 << 20
	directReferenceFileLimit    = 16 << 20
)

// decodeDirectRequestBody 解析画布发来的请求体。
//
// 账号渠道（登录后）下，画布在带参考图时发的是 multipart——图片放在文件字段里
// （见 canvas_task.go 的 stripCanvasTaskMultipartFields），所以不能只按 JSON 解析，
// 否则图片编辑会直接以"只接受 JSON"失败。文件字段统一转成 data URI：
// Fal 的模型输入普遍接受 data URI；Replicate 官方建议单文件 1MB 以内，
// 超出时由平台自行拒绝（错误原文现在会如实带出来）。
func decodeDirectRequestBody(raw []byte, contentType string, label string) (map[string]any, error) {
	if !strings.HasPrefix(strings.ToLower(strings.TrimSpace(contentType)), "multipart/form-data") {
		return decodeDirectBodyObject(raw, label)
	}
	if len(raw) > directReferenceRequestLimit {
		return nil, fmt.Errorf("%s 渠道的请求体超过 %dMB", label, directReferenceRequestLimit>>20)
	}
	_, params, err := mime.ParseMediaType(contentType)
	if err != nil {
		return nil, errors.New(label + " 渠道的请求体格式无法解析")
	}
	form, err := multipart.NewReader(bytes.NewReader(raw), params["boundary"]).ReadForm(directReferenceRequestLimit)
	if err != nil {
		return nil, errors.New(label + " 渠道的请求体格式无法解析")
	}
	defer form.RemoveAll()

	body := map[string]any{}
	appendField := func(key string, value any) {
		if strings.HasPrefix(key, "_canvas_") {
			return
		}
		if existing, ok := body[key]; ok {
			if list, ok := existing.([]any); ok {
				body[key] = append(list, value)
				return
			}
			body[key] = []any{existing, value}
			return
		}
		body[key] = value
	}
	for key, values := range form.Value {
		for _, value := range values {
			appendField(key, parseDirectFormValue(value))
		}
	}
	for key, headers := range form.File {
		for _, header := range headers {
			data, mimeType, err := readDirectFormFile(header)
			if err != nil {
				return nil, err
			}
			if mimeType == "" || strings.EqualFold(mimeType, "application/octet-stream") {
				// 画布上传的文件常不带精确的类型，按扩展名补一个，避免一律降级成 octet-stream。
				if byExtension := mime.TypeByExtension(filepath.Ext(header.Filename)); byExtension != "" {
					mimeType = byExtension
				}
			}
			if mimeType == "" {
				mimeType = "application/octet-stream"
			}
			appendField(key, "data:"+mimeType+";base64,"+base64.StdEncoding.EncodeToString(data))
		}
	}
	return body, nil
}

func parseDirectFormValue(value string) any {
	text := strings.TrimSpace(value)
	if text == "" {
		return ""
	}
	var parsed any
	if json.Unmarshal([]byte(text), &parsed) == nil {
		return parsed
	}
	return text
}

func readDirectFormFile(header *multipart.FileHeader) ([]byte, string, error) {
	file, err := header.Open()
	if err != nil {
		return nil, "", errors.New("读取参考素材失败：" + header.Filename)
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, directReferenceFileLimit+1))
	if err != nil {
		return nil, "", errors.New("读取参考素材失败：" + header.Filename)
	}
	if int64(len(data)) > directReferenceFileLimit {
		return nil, "", fmt.Errorf("参考素材 %s 超过 %dMB，请改用公网地址", header.Filename, directReferenceFileLimit>>20)
	}
	return data, strings.TrimSpace(header.Header.Get("Content-Type")), nil
}

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

// readDirectReferences 收集参考素材；兼容单值与数组两种写法。
//
// 两种模式下字段值的形态不同，都要认：
//   - 本地直连：浏览器上传前先放占位符（direct-reference.invalid），由浏览器换成真实地址；
//   - 账号渠道：画布后端代理，传进来的已经是画布服务端地址或内联 data URI。
//
// 早先只认占位符，导致账号渠道下的图片编辑一律报"需要至少一张参考图"。
func readDirectReferences(body map[string]any, key string) []string {
	value, ok := body[key]
	if !ok {
		return nil
	}
	collect := func(item any) []string {
		text := readDirectString(item)
		if text == "" || !isDirectReferenceValue(text) {
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

// isDirectReferenceValue 判断字段值是不是参考素材：占位符、公网地址或内联数据。
func isDirectReferenceValue(text string) bool {
	text = strings.TrimSpace(text)
	if text == "" {
		return false
	}
	lower := strings.ToLower(text)
	// data URI 的具体 MIME 不必苛刻：字段名（image / video_reference[] …）已经说明用途。
	if strings.HasPrefix(lower, "data:") {
		return true
	}
	return strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://")
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
