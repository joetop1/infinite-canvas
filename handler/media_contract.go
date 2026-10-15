package handler

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/tigerowo/infinite-canvas/service"
)

func normalizeContractDirectBody(input aiProtocolRequest) ([]byte, error) {
	body, err := decodeDirectRequestBody(input.body, input.contentType, "模型规范")
	if err != nil {
		return nil, err
	}
	spec := parseDirectModelSpec(input.modelName)
	if len(spec.Fields) > 0 {
		return nil, fmt.Errorf("字段覆写尚未纳入官方规范，请使用渠道参数转译配置")
	}
	out, err := service.PrepareMediaContractInput(input.channel, input.modelName, input.endpoint, body, spec.Params)
	if err != nil {
		return nil, err
	}
	if service.IsReplicateChannel(input.channel) {
		_, version, err := splitReplicateModel(spec.ModelPath, spec.Version)
		if err != nil {
			return nil, err
		}
		wrapped := map[string]any{"input": out}
		if version == "" {
			version = service.MediaContractVersion(input.channel, input.modelName)
		}
		if version == "" {
			return nil, fmt.Errorf("Replicate 未返回与规范对应的模型版本，尚未提交")
		}
		if version != "" {
			wrapped["version"] = version
		}
		return json.Marshal(wrapped)
	}
	return json.Marshal(out)
}

func AdminChannelContracts(w http.ResponseWriter, r *http.Request) {
	var input adminChannelActionRequest
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&input) != nil || len(input.Channel.Models) > 30 {
		Fail(w, "单次最多同步 30 个模型")
		return
	}
	if !service.IsFalChannel(input.Channel) && !service.IsReplicateChannel(input.Channel) && !service.IsArkChannel(input.Channel) && input.Channel.Protocol != "openai" {
		Fail(w, "此渠道尚未接入统一模型规范")
		return
	}
	result, err := service.AdminMediaContracts(input.Index, input.Channel)
	if err != nil {
		FailError(w, err)
		return
	}
	OK(w, result)
}
