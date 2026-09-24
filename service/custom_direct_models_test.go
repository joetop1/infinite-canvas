package service

import (
	"errors"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/tigerowo/infinite-canvas/model"
)

// Fal / Replicate 的模型发现必须命中各自的内置清单，不能回落到 OpenAI 的 /models。
//
// 这条断言针对一个真实踩过的坑：协议在 modelProtocolRegistry 里注册了，
// 但忘了加进 modelDiscoveryRules，于是 matchModelProtocol 回落成 OpenAI 适配器，
// 请求打到 {baseUrl}/models（即 queue.fal.run/models），管理后台只显示「读取模型失败：404」。
//
// 判定方式：把上游 HTTP 客户端换成"一旦被调用就报错"的桩。清单是纯静态的，
// 只要 transport 被触发，就说明发生了回落，测试立刻失败。
func TestCustomDirectModelDiscoveryDoesNotFallBackToOpenAI(t *testing.T) {
	stubProtocolAdminHTTP(t, func(*http.Request) (*http.Response, error) {
		t.Error("模型发现发起了网络请求，说明回落到 OpenAI 的 /models 了")
		return nil, errors.New("network forbidden")
	})
	tests := []struct {
		name, protocol, baseURL string
		want                    []string
	}{
		{"Fal", "fal", "https://queue.fal.run", FalModels()},
		{"Fal 大小写与空白", "  FAL  ", "https://queue.fal.run", FalModels()},
		{"Replicate", "replicate", "https://api.replicate.com/v1", ReplicateModels()},
		{"Replicate 大小写与空白", " Replicate ", "https://api.replicate.com/v1", ReplicateModels()},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := AdminChannelModels(nil, model.ModelChannel{Protocol: test.protocol, BaseURL: test.baseURL, APIKey: "test-key"})
			if err != nil || !reflect.DeepEqual(got, test.want) {
				t.Fatalf("got %v, %v; want %d 个内置模型", got, err, len(test.want))
			}
		})
	}
}

// 「测试渠道」不能拿 OpenAI 的 chat/completions 去测 Fal / Replicate——
// 两个平台都没有这个端点，只会白报一个错。
func TestCustomDirectChannelConfigTestDoesNotCallOpenAI(t *testing.T) {
	stubProtocolAdminHTTP(t, func(*http.Request) (*http.Response, error) {
		t.Error("渠道测试发起了网络请求，说明回落到 OpenAI 的 chat/completions 了")
		return nil, errors.New("network forbidden")
	})
	for _, protocol := range []string{"fal", "replicate"} {
		got, err := AdminTestChannelModel(nil, model.ModelChannel{Protocol: protocol, BaseURL: "https://api.invalid", APIKey: "test-key"}, "some-model")
		if err != nil || !strings.Contains(got, "创作台") {
			t.Errorf("%s: got %q, %v; want 引导到创作台验证的提示", protocol, got, err)
		}
	}
}

// 清单自身的格式约束。这些名字最终会原样拼进请求地址，写错一个字符就等于让用户白跑一次生成。
func TestCustomDirectModelListShape(t *testing.T) {
	fal := FalModels()
	if len(fal) == 0 {
		t.Fatal("Fal 模型清单为空")
	}
	seen := make(map[string]bool, len(fal))
	for _, name := range fal {
		if seen[name] {
			t.Errorf("Fal 清单有重复项：%s", name)
		}
		seen[name] = true
		if name != strings.TrimSpace(name) || strings.HasPrefix(name, "/") || strings.ContainsAny(name, " ?#") {
			t.Errorf("Fal 模型名格式异常：%q", name)
		}
		if !strings.Contains(name, "/") {
			t.Errorf("Fal 模型名应形如 fal-ai/xxx：%q", name)
		}
	}

	replicate := ReplicateModels()
	if len(replicate) == 0 {
		t.Fatal("Replicate 模型清单为空")
	}
	seen = make(map[string]bool, len(replicate))
	for _, name := range replicate {
		if seen[name] {
			t.Errorf("Replicate 清单有重复项：%s", name)
		}
		seen[name] = true
		// 内置项只放官方模型（owner/name，不带版本号）；社区模型要带 :version，由用户手填。
		if strings.Count(name, "/") != 1 || strings.ContainsAny(name, " ?#:") {
			t.Errorf("Replicate 模型名应形如 owner/name：%q", name)
		}
	}
}
