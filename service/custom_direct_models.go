package service

// [CUSTOM] Fal.ai 与 Replicate 的内置模型清单。
//
// 为什么不做成「实时拉取」：
//   - Fal：queue.fal.run 下没有 /models 路由（旧实现打到这里拿到 404）。
//     api.fal.ai/v1/models 虽然可以匿名读取，但不支持 category / q / search 过滤，
//     只能靠 limit + cursor 翻页，模型总量以千计（翻到第 14 页 1400 条仍未结束）。
//     整份拉下来既慢，下拉框里几千项也无法使用。
//   - Replicate：GET /v1/models 只返回当前账号自建的模型，拿不到公开目录。
//
// 因此沿用 KIE / MiMo / MiniMax 的内置清单做法，列出常用的主流模型；
// 用户仍可在渠道里手动补充任意模型名（选择弹窗自带输入框）。
//
// 清单内的名称均已逐一验证存在：
//   - Fal 用 fal.ai/models/{endpoint_id} 页面探测，反向对照（故意写错的名字返回 404）确认有效；
//   - Replicate 同理用 replicate.com/{owner}/{name} 探测，luma/ray 等已失效项已剔除。
//
// 维护：模型下线时这里会失效，重新验证后替换即可。

// FalModels 返回 Fal.ai 的常用模型，元素是 endpoint id（提交地址为 {baseUrl}/{endpoint id}）。
// 顺序按用途分组，未做字典序排序——分组比字母序更便于在弹窗里定位。
func FalModels() []string {
	return []string{
		// 文生图
		"fal-ai/flux/dev",
		"fal-ai/flux/schnell",
		"fal-ai/flux-pro",
		"fal-ai/flux-pro/v1.1",
		"fal-ai/flux-pro/v1.1-ultra",
		"fal-ai/flux-lora",
		"fal-ai/fast-sdxl",
		"fal-ai/stable-diffusion-v35-large",
		"fal-ai/nano-banana-pro",
		"fal-ai/qwen-image",
		// 图生图 / 编辑
		"fal-ai/nano-banana-pro/edit",
		"fal-ai/nano-banana/edit",
		// 文生视频
		"fal-ai/veo3/fast",
		"fal-ai/kling-video/v2.1/master/text-to-video",
		"fal-ai/kling-video/v1.6/standard/text-to-video",
		"fal-ai/kling-video/v1.5/pro/text-to-video",
		"fal-ai/minimax/hailuo-02/standard/text-to-video",
		"fal-ai/minimax/video-01",
		"fal-ai/wan/v2.2-a14b/text-to-video",
		"fal-ai/bytedance/seedance/v1/pro/text-to-video",
		// 图生视频
		"fal-ai/kling-video/v1.5/pro/image-to-video",
		"fal-ai/minimax/video-01/image-to-video",
		"fal-ai/wan/v2.2-a14b/image-to-video",
		"fal-ai/bytedance/seedance/v1/pro/image-to-video",
	}
}

// ReplicateModels 返回 Replicate 的常用模型，元素是 owner/name。
//
// 这里以官方模型为主，因为官方模型可以直接 POST /v1/models/{owner}/{name}/predictions，
// 无需附带版本号；社区模型必须在名字后带 :version（或 ?version=hash），
// 版本号不便内置（会随模型更新失效），需要时由用户手动填写。
func ReplicateModels() []string {
	return []string{
		// 文生图
		"black-forest-labs/flux-schnell",
		"black-forest-labs/flux-dev",
		"black-forest-labs/flux-1.1-pro",
		"black-forest-labs/flux-2-pro",
		"black-forest-labs/flux-2-flex",
		"black-forest-labs/flux-kontext-pro",
		"black-forest-labs/flux-kontext-max",
		"google/imagen-4",
		"bytedance/seedream-3",
		"bytedance/seedream-4",
		"openai/gpt-image-1",
		"qwen/qwen-image-edit",
		"stability-ai/stable-diffusion-3.5-large",
		"recraft-ai/recraft-v3",
		"ideogram-ai/ideogram-v3-turbo",
		"prunaai/flux-fast",
		// 视频
		"google/veo-3-fast",
		"google/veo-3.1",
		"kwaivgi/kling-v2.1",
		"kwaivgi/kling-v2.5-turbo-pro",
		"minimax/video-01",
		"minimax/hailuo-02",
		"wan-video/wan-2.5-t2v",
		"wan-video/wan-2.5-i2v",
		"bytedance/seedance-1-pro",
	}
}
