package service

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"sync"

	"github.com/santhosh-tekuri/jsonschema/v5"
	"github.com/tigerowo/infinite-canvas/model"
)

//go:embed native-seedance-schema.json
var nativeSeedanceSchemaJSON string
var nativeSeedanceSchemas sync.Map

func nativeSeedanceContract(channel model.ModelChannel, name string) *model.MediaContract {
	if !IsArkChannel(channel) && channel.Protocol != "openai" {
		return nil
	}
	key := strings.NewReplacer(".", "-", "_", "-").Replace(strings.ToLower(name))
	if !strings.HasPrefix(key, "doubao-seedance-2-") && !strings.HasPrefix(key, "seedance-2-") {
		return nil
	}
	contract := &model.MediaContract{Version: 1, Status: "unadapted", Reason: "该 Seedance 原生版本的渠道规范尚未核对", Capability: "video", Source: "https://docs.byteplus.com/en/docs/modelark/create-video-generation-task-api"}
	if !strings.Contains(key, "seedance-2-0") {
		return contract
	}
	resolutions := []any{"480p", "720p", "1080p"}
	if strings.Contains(key, "fast") || strings.Contains(key, "mini") {
		resolutions = []any{"720p"}
	}
	min, max := 4.0, 15.0
	contract.Status, contract.Reason = "ready", ""
	contract.MaxReferences, contract.ReferenceMinSeconds, contract.ReferenceMaxSeconds, contract.ReferenceTotalSeconds = 12, 2, 15, 15
	contract.Bindings = map[string]model.MediaContractField{
		"images": {Name: "content.image_url", Type: "array", MaxItems: 9}, "videos": {Name: "content.video_url", Type: "array", MaxItems: 3}, "audios": {Name: "content.audio_url", Type: "array", MaxItems: 3},
		"first_frame": {Name: "content.first_frame", Type: "string", MaxItems: 1}, "last_frame": {Name: "content.last_frame", Type: "string", MaxItems: 1},
		"duration":   {Name: "duration", Type: "integer", Min: &min, Max: &max, Default: 5},
		"resolution": {Name: "resolution", Type: "string", Values: resolutions, Default: "720p"},
		"ratio":      {Name: "ratio", Type: "string", Values: []any{"adaptive", "16:9", "9:16", "1:1", "4:3", "3:4", "21:9"}, Default: "16:9"},
		"audio":      {Name: "generate_audio", Type: "boolean", Default: true},
	}
	return contract
}

func ValidateNativeSeedanceContract(channel model.ModelChannel, name string, body []byte) error {
	if !channel.StrictContracts {
		return nil
	}
	contract := nativeSeedanceContract(channel, name)
	if contract == nil {
		return nil
	}
	if contract.Status != "ready" {
		return fmt.Errorf("模型尚未适配：%s", contract.Reason)
	}
	var input map[string]any
	if err := json.Unmarshal(body, &input); err != nil {
		return fmt.Errorf("Seedance 请求不是有效 JSON")
	}
	payload := input
	if metadata, ok := input["metadata"].(map[string]any); ok {
		payload = metadata
	}
	content, _ := payload["content"].([]any)
	canonical := map[string]any{"content": content, "resolution": payload["resolution"], "ratio": payload["ratio"]}
	if prompt := contractText(input["prompt"]); prompt != "" {
		content = append([]any{map[string]any{"type": "text", "text": prompt}}, content...)
		canonical["content"] = content
	}
	seconds := payload["duration"]
	if seconds == nil {
		seconds = input["seconds"]
	}
	if value, err := strconv.ParseFloat(contractText(seconds), 64); err == nil {
		canonical["duration"] = value
	}
	for _, key := range []string{"generate_audio", "watermark"} {
		if value, ok := payload[key]; ok {
			canonical[key] = value
		}
	}
	if schema, ok := nativeSeedanceSchemas.Load("2.0"); ok {
		if err := schema.(*jsonschema.Schema).Validate(canonical); err != nil {
			return fmt.Errorf("Seedance 参数不符合规范：%s", contractValidationMessage(err))
		}
	} else {
		schema, err := jsonschema.CompileString("https://canvas.invalid/native-seedance.json", nativeSeedanceSchemaJSON)
		if err != nil {
			return fmt.Errorf("Seedance 输入规范无法编译")
		}
		nativeSeedanceSchemas.Store("2.0", schema)
		if err := schema.Validate(canonical); err != nil {
			return fmt.Errorf("Seedance 参数不符合规范：%s", contractValidationMessage(err))
		}
	}
	images, videos, audios, first, last := 0, 0, 0, 0, 0
	for _, value := range content {
		item, _ := value.(map[string]any)
		switch item["role"] {
		case "reference_image":
			images++
		case "reference_video":
			videos++
		case "reference_audio":
			audios++
		case "first_frame":
			first++
		case "last_frame":
			last++
		}
	}
	if images > 9 || videos > 3 || audios > 3 || images+videos+audios > 12 || first > 1 || last > 1 {
		return fmt.Errorf("Seedance 参考素材数量超出模型限制")
	}
	if first+last > 0 && images+videos+audios > 0 || last > 0 && first == 0 {
		return fmt.Errorf("Seedance 首尾帧与普通参考素材不能混用，尾帧需要同时指定首帧")
	}
	if audios > 0 && images+videos == 0 {
		return fmt.Errorf("Seedance 2.0 参考音频需要同时提供图片或视频")
	}
	resolution := contractText(canonical["resolution"])
	allowed := false
	for _, value := range contract.Bindings["resolution"].Values {
		allowed = allowed || resolution == value
	}
	if !allowed {
		return fmt.Errorf("此 Seedance 版本不支持当前清晰度")
	}
	return nil
}
