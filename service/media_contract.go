package service

import (
	"bytes"
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/santhosh-tekuri/jsonschema/v5"
	"github.com/tigerowo/infinite-canvas/model"
	"golang.org/x/sync/singleflight"
)

type mediaContractEntry struct {
	Description model.MediaContract
	Schema      *jsonschema.Schema
	Properties  map[string]*jsonschema.Schema
	Expires     time.Time
}

var mediaContracts = struct {
	sync.RWMutex
	Entries map[string]*mediaContractEntry
	Loading map[string]bool
}{Entries: map[string]*mediaContractEntry{}, Loading: map[string]bool{}}
var mediaContractLoads singleflight.Group
var mediaContractSlots = make(chan struct{}, 3)

// Semantic rules not expressed by provider JSON Schema are reviewed as data.
//
//go:embed media-contract-policies.json
var mediaContractPolicyData []byte
var mediaContractPolicies = func() map[string]model.MediaContract {
	policies := map[string]model.MediaContract{}
	if err := json.Unmarshal(mediaContractPolicyData, &policies); err != nil {
		panic(err)
	}
	return policies
}()

func usesMediaContract(channel model.ModelChannel) bool {
	return IsFalChannel(channel) || IsReplicateChannel(channel)
}

func mediaContractKey(channel model.ModelChannel, name string) string {
	provider := "fal"
	if IsReplicateChannel(channel) {
		provider = "replicate:" + strings.TrimRight(channel.BaseURL, "/")
	}
	return provider + ":" + mediaContractModelID(channel, name)
}

func mediaContractModelID(channel model.ModelChannel, name string) string {
	parts := strings.SplitN(name, "?", 2)
	path := parts[0]
	if IsReplicateChannel(channel) && !strings.Contains(path, ":") && len(parts) == 2 {
		query, _ := url.ParseQuery(parts[1])
		if version := query.Get("version"); version != "" {
			path += ":" + version
		}
	}
	return path
}

func MediaContractVersion(channel model.ModelChannel, name string) string {
	return loadMediaContract(channel, name).Description.ProviderVersion
}

func cachedMediaContract(channel model.ModelChannel, name string) *mediaContractEntry {
	mediaContracts.RLock()
	defer mediaContracts.RUnlock()
	entry := mediaContracts.Entries[mediaContractKey(channel, name)]
	if entry != nil && time.Now().Before(entry.Expires) {
		return entry
	}
	return nil
}

func loadMediaContract(channel model.ModelChannel, name string) *mediaContractEntry {
	if entry := cachedMediaContract(channel, name); entry != nil {
		return entry
	}
	value, _, _ := mediaContractLoads.Do(mediaContractKey(channel, name), func() (any, error) {
		if entry := cachedMediaContract(channel, name); entry != nil {
			return entry, nil
		}
		mediaContractSlots <- struct{}{}
		defer func() { <-mediaContractSlots }()
		ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
		defer cancel()
		entry := fetchMediaContract(ctx, channel, name)
		mediaContracts.Lock()
		mediaContracts.Entries[mediaContractKey(channel, name)] = entry
		mediaContracts.Unlock()
		return entry, nil
	})
	return value.(*mediaContractEntry)
}

// PublishedMediaMetadata starts a bounded background refresh; settings reads stay fast.
func PublishedMediaMetadata(channel model.ModelChannel, name string) json.RawMessage {
	if !channel.StrictContracts {
		data := map[string]any{}
		_ = json.Unmarshal(channel.ModelMetadata[name], &data)
		if data != nil {
			delete(data, "generation_contract")
			result, _ := json.Marshal(data)
			return result
		}
		return channel.ModelMetadata[name]
	}
	if native := nativeSeedanceContract(channel, name); native != nil {
		return mediaContractMetadata(channel.ModelMetadata[name], *native)
	}
	if !usesMediaContract(channel) {
		return channel.ModelMetadata[name]
	}
	entry := cachedMediaContract(channel, name)
	if entry == nil {
		key := mediaContractKey(channel, name)
		mediaContracts.Lock()
		if !mediaContracts.Loading[key] {
			mediaContracts.Loading[key] = true
			go func() {
				loadMediaContract(channel, name)
				mediaContracts.Lock()
				delete(mediaContracts.Loading, key)
				mediaContracts.Unlock()
			}()
		}
		mediaContracts.Unlock()
		return mediaContractMetadata(channel.ModelMetadata[name], model.MediaContract{Version: 1, Status: "syncing", Reason: "正在同步官方模型规范，请稍后刷新"})
	}
	return mediaContractMetadata(channel.ModelMetadata[name], entry.Description)
}

func mediaContractMetadata(raw json.RawMessage, contract model.MediaContract) json.RawMessage {
	data := map[string]any{}
	_ = json.Unmarshal(raw, &data)
	if data == nil {
		data = map[string]any{}
	}
	data["generation_contract"] = contract
	result, _ := json.Marshal(data)
	return result
}

func AdminMediaContracts(index *int, channel model.ModelChannel) (ChannelModelCatalog, error) {
	resolved, err := resolveAdminChannel(index, channel)
	if err != nil {
		return ChannelModelCatalog{}, err
	}
	mediaContracts.Lock()
	for _, name := range resolved.Models {
		delete(mediaContracts.Entries, mediaContractKey(resolved, name))
	}
	mediaContracts.Unlock()
	return SyncMediaContracts(resolved), nil
}

func SyncMediaContracts(channel model.ModelChannel) ChannelModelCatalog {
	result := ChannelModelCatalog{Models: channel.Models, ModelCapabilities: map[string]string{}, ModelMetadata: map[string]json.RawMessage{}}
	var mu sync.Mutex
	var workers sync.WaitGroup
	for _, name := range channel.Models {
		workers.Add(1)
		go func(name string) {
			defer workers.Done()
			var descriptor model.MediaContract
			if native := nativeSeedanceContract(channel, name); native != nil {
				descriptor = *native
			} else if usesMediaContract(channel) {
				descriptor = loadMediaContract(channel, name).Description
			} else {
				return
			}
			mu.Lock()
			defer mu.Unlock()
			result.ModelMetadata[name] = mediaContractMetadata(channel.ModelMetadata[name], descriptor)
			if descriptor.Capability != "" {
				result.ModelCapabilities[name] = descriptor.Capability
			}
		}(name)
	}
	workers.Wait()
	return result
}

func fetchMediaContract(ctx context.Context, channel model.ModelChannel, name string) *mediaContractEntry {
	entry := &mediaContractEntry{Description: model.MediaContract{Version: 1, Status: "unadapted", FetchedAt: time.Now().UTC().Format(time.RFC3339)}, Expires: time.Now().Add(5 * time.Minute)}
	fail := func(reason string) *mediaContractEntry { entry.Description.Reason = reason; return entry }
	path := mediaContractModelID(channel, name)
	var endpoint, inputRef string
	if IsFalChannel(channel) {
		endpoint = "https://fal.ai/api/openapi/queue/openapi.json?" + url.Values{"endpoint_id": {path}}.Encode()
	} else {
		parts := strings.SplitN(path, ":", 2)
		endpoint = BuildModelChannelURL(channel, "/models/"+parts[0])
		if len(parts) == 2 {
			endpoint += "/versions/" + url.PathEscape(parts[1])
		}
		inputRef = "#/components/schemas/Input"
	}
	entry.Description.Source = endpoint
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return fail("官方模型规范地址无效")
	}
	request.Header.Set("Accept", "application/json")
	if IsFalChannel(channel) && strings.HasPrefix(endpoint, "https://api.fal.ai/") {
		request.Header.Set("Authorization", FalAuthorizationHeader(channel.APIKey))
	}
	if IsReplicateChannel(channel) {
		SetModelChannelAuthHeader(request, channel)
	}
	response, err := SafeProxyHTTPClient().Do(request)
	if err != nil {
		return fail("官方模型规范暂时无法读取，请同步后重试")
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fail(fmt.Sprintf("官方模型规范返回 HTTP %d，请检查渠道后重新同步", response.StatusCode))
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, (2<<20)+1))
	if err != nil || len(data) > 2<<20 {
		return fail("官方模型规范读取不完整或过大")
	}
	var document map[string]any
	if json.Unmarshal(data, &document) != nil {
		return fail("官方模型规范不是有效 JSON")
	}
	if IsFalChannel(channel) {
		if items, ok := document["models"].([]any); ok {
			if len(items) != 1 {
				return fail("官方模型规范没有返回唯一匹配项")
			}
			item, _ := items[0].(map[string]any)
			document, _ = item["openapi"].(map[string]any)
			if document == nil {
				return fail("官方目录尚未提供该模型的 OpenAPI 输入规范")
			}
		}
		inputRef, _ = contractMapPath(document, "paths", "/"+path, "post", "requestBody", "content", "application/json", "schema", "$ref").(string)
		if inputRef == "" {
			return fail("该模型没有可识别的标准输入 Schema，需人工适配")
		}
		category, _ := contractMapPath(document, "info", "x-fal-metadata", "category").(string)
		if strings.Contains(category, "video") {
			entry.Description.Capability = "video"
		} else if strings.Contains(category, "image") {
			entry.Description.Capability = "image"
		}
	} else {
		if version, ok := document["latest_version"].(map[string]any); ok {
			document = version
		}
		entry.Description.ProviderVersion, _ = document["id"].(string)
		document, _ = document["openapi_schema"].(map[string]any)
		if document == nil {
			return fail("Replicate 尚未提供此版本的输入 Schema")
		}
		entry.Description.Capability = channel.ModelCapabilities[name]
		if entry.Description.Capability == "" {
			if isVideoModelName(name) {
				entry.Description.Capability = "video"
			} else if isImageModelName(name) {
				entry.Description.Capability = "image"
			}
		}
	}
	if entry.Description.Capability != "image" && entry.Description.Capability != "video" {
		return fail("该模型尚未适配图片或视频生成任务")
	}
	root := map[string]any{"$schema": "https://json-schema.org/draft/2020-12/schema", "$ref": inputRef, "components": document["components"]}
	// OpenAPI 3.0 nullable is converted before compiling with the JSON Schema library.
	normalizeContractNullable(root)
	encoded, _ := json.Marshal(root)
	sum := sha256.Sum256(encoded)
	entry.Description.Revision = hex.EncodeToString(sum[:])
	compiler := jsonschema.NewCompiler()
	compiler.ExtractAnnotations = true
	compiler.LoadURL = func(string) (io.ReadCloser, error) { return nil, errors.New("模型规范不允许读取外部引用") }
	if compiler.AddResource("https://canvas.invalid/input.json", bytes.NewReader(encoded)) != nil {
		return fail("模型规范无法载入")
	}
	entry.Schema, err = compiler.Compile("https://canvas.invalid/input.json")
	if err != nil {
		return fail("模型输入规范无法编译，需人工适配")
	}
	entry.Properties = contractProperties(entry.Schema, 0)
	entry.Description.Bindings = inferContractBindings(entry.Properties)
	requiredNames := contractRequired(entry.Schema, 0)
	for role, field := range entry.Description.Bindings {
		field.Required = slices.Contains(requiredNames, field.Name)
		entry.Description.Bindings[role] = field
	}
	if _, ok := entry.Description.Bindings["prompt"]; !ok {
		return fail("该模型未提供标准提示词输入，需人工适配")
	}
	bound := map[string]bool{}
	for _, field := range entry.Description.Bindings {
		bound[field.Name] = true
	}
	for _, required := range contractRequired(entry.Schema, 0) {
		field := effectiveContractSchema(entry.Properties[required], 0)
		if !bound[required] && (field == nil || field.Default == nil) {
			return fail("必填参数 " + required + " 尚未适配，禁止直接生成")
		}
	}
	entry.Description.RequiresReference = strings.Contains(path, "reference-to-video") || strings.HasSuffix(path, "/image-to-video") || strings.HasSuffix(path, "/edit")
	if entry.Description.RequiresReference && entry.Description.Bindings["images"].Name == "" && entry.Description.Bindings["videos"].Name == "" && entry.Description.Bindings["audios"].Name == "" {
		return fail("参考素材需要结构化输入或专属映射，尚未适配")
	}
	if strings.HasSuffix(path, "/image-to-video") {
		entry.Description.RequiresReference = true
	}
	if policy, ok := mediaContractPolicies[path]; ok {
		entry.Description.NativeAudio = policy.NativeAudio
		entry.Description.MaxReferences = policy.MaxReferences
		entry.Description.ReferenceMinSeconds = policy.ReferenceMinSeconds
		entry.Description.ReferenceMaxSeconds = policy.ReferenceMaxSeconds
		entry.Description.ReferenceTotalSeconds = policy.ReferenceTotalSeconds
	}
	if entry.Description.Bindings["videos"].Name != "" && entry.Description.ReferenceMaxSeconds == 0 {
		return fail("参考视频的业务限制尚未核对，需完成适配后启用")
	}
	entry.Description.Status, entry.Description.Reason = "ready", ""
	entry.Expires = time.Now().Add(24 * time.Hour)
	return entry
}

func contractMapPath(value map[string]any, keys ...string) any {
	var item any = value
	for _, key := range keys {
		next, ok := item.(map[string]any)
		if !ok {
			return nil
		}
		item = next[key]
	}
	return item
}

func normalizeContractNullable(value any) {
	switch value := value.(type) {
	case map[string]any:
		if value["nullable"] == true {
			if kind, ok := value["type"].(string); ok {
				value["type"] = []any{kind, "null"}
			}
			delete(value, "nullable")
		}
		for _, child := range value {
			normalizeContractNullable(child)
		}
	case []any:
		for _, child := range value {
			normalizeContractNullable(child)
		}
	}
}

func effectiveContractSchema(schema *jsonschema.Schema, depth int) *jsonschema.Schema {
	if schema == nil || depth > 12 {
		return nil
	}
	if schema.Ref != nil {
		resolved := effectiveContractSchema(schema.Ref, depth+1)
		if resolved != nil && schema.Default != nil {
			copied := *resolved
			copied.Default = schema.Default
			return &copied
		}
		return resolved
	}
	if len(schema.Types) > 0 {
		return schema
	}
	for _, option := range append(append([]*jsonschema.Schema{}, schema.AnyOf...), schema.OneOf...) {
		item := effectiveContractSchema(option, depth+1)
		if item != nil && !slices.Equal(item.Types, []string{"null"}) {
			if schema.Default != nil {
				copied := *item
				copied.Default = schema.Default
				return &copied
			}
			return item
		}
	}
	return schema
}

func contractTypeSchema(schema *jsonschema.Schema, kind string, depth int) *jsonschema.Schema {
	if schema == nil || depth > 12 {
		return nil
	}
	if slices.Contains(schema.Types, kind) {
		return schema
	}
	if schema.Ref != nil {
		if result := contractTypeSchema(schema.Ref, kind, depth+1); result != nil {
			return result
		}
	}
	for _, item := range append(append([]*jsonschema.Schema{}, schema.AnyOf...), schema.OneOf...) {
		if result := contractTypeSchema(item, kind, depth+1); result != nil {
			return result
		}
	}
	return nil
}

func contractProperties(schema *jsonschema.Schema, depth int) map[string]*jsonschema.Schema {
	result := map[string]*jsonschema.Schema{}
	if schema == nil || depth > 12 {
		return result
	}
	if schema.Ref != nil {
		for key, value := range contractProperties(schema.Ref, depth+1) {
			result[key] = value
		}
	}
	for _, child := range schema.AllOf {
		for key, value := range contractProperties(child, depth+1) {
			result[key] = value
		}
	}
	for key, value := range schema.Properties {
		result[key] = value
	}
	return result
}

func contractRequired(schema *jsonschema.Schema, depth int) []string {
	if schema == nil || depth > 12 {
		return nil
	}
	result := append([]string{}, schema.Required...)
	if schema.Ref != nil {
		result = append(result, contractRequired(schema.Ref, depth+1)...)
	}
	for _, child := range schema.AllOf {
		result = append(result, contractRequired(child, depth+1)...)
	}
	return result
}

func inferContractBindings(properties map[string]*jsonschema.Schema) map[string]model.MediaContractField {
	result := map[string]model.MediaContractField{}
	aliases := map[string][]string{
		"prompt": {"prompt"}, "images": {"reference_image_urls", "image_urls", "image_input", "input_images", "images", "image", "input_image", "image_url"},
		"videos": {"reference_video_urls", "video_urls", "video_input", "videos", "video", "video_url"}, "audios": {"reference_audio_urls", "audio_urls", "audio_input", "audios", "audio", "audio_url"},
		"first_frame": {"first_frame_url", "start_image_url", "start_frame_url"}, "last_frame": {"last_frame_url", "end_image_url", "end_frame_url", "last_frame_image"},
		"duration": {"duration", "seconds"}, "resolution": {"resolution"}, "ratio": {"aspect_ratio"}, "size": {"image_size"},
		"width": {"width"}, "height": {"height"}, "count": {"num_images", "num_outputs", "n"}, "audio": {"generate_audio"}, "keep_audio": {"keep_audio"}, "negative_prompt": {"negative_prompt"}, "quality": {"quality"},
	}
	for role, names := range aliases {
		for _, name := range names {
			schema := effectiveContractSchema(properties[name], 0)
			if role == "size" {
				if object := contractTypeSchema(properties[name], "object", 0); object != nil {
					schema = object
				}
			}
			if role == "images" || role == "videos" || role == "audios" {
				if array := contractTypeSchema(properties[name], "array", 0); array != nil {
					schema = array
				}
			}
			if schema == nil {
				continue
			}
			kind := ""
			for _, candidate := range schema.Types {
				if candidate != "null" {
					kind = candidate
					break
				}
			}
			if kind == "" || kind == "object" && role != "size" {
				continue
			}
			if kind == "array" && role != "images" && role != "videos" && role != "audios" {
				continue
			}
			if kind == "array" {
				item := schema.Items2020
				if item == nil {
					item, _ = schema.Items.(*jsonschema.Schema)
				}
				if item = effectiveContractSchema(item, 0); item != nil && !slices.Contains(item.Types, "string") {
					continue
				}
			}
			field := model.MediaContractField{Name: name, Type: kind, Default: schema.Default, Values: schema.Enum}
			if schema.Minimum != nil {
				value, _ := schema.Minimum.Float64()
				field.Min = &value
			}
			if schema.Maximum != nil {
				value, _ := schema.Maximum.Float64()
				field.Max = &value
			}
			if kind == "array" && schema.MaxItems >= 0 {
				field.MaxItems = schema.MaxItems
			}
			if (role == "images" || role == "videos" || role == "audios") && kind == "string" {
				field.MaxItems = 1
			}
			result[role] = field
			break
		}
	}
	if images, ok := result["images"]; ok && images.Name != "image_url" && properties["image_url"] != nil {
		schema := effectiveContractSchema(properties["image_url"], 0)
		if schema != nil && slices.Contains(schema.Types, "string") {
			result["first_frame"] = model.MediaContractField{Name: "image_url", Type: "string", MaxItems: 1}
		}
	}
	return result
}
