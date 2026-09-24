# Fal.ai / Replicate 渠道接入说明

本文是**新增功能**（不改上游既有行为）的使用说明。两个平台都属于"直连协议"：
请求从浏览器直接发往平台，画布后端只负责**把画布参数转译成平台报文**。

---

## 一、为什么模型名要带参数

Fal 和 Replicate 与 OpenAI 不同：**每个模型有各自的输入 schema**。
同样是文生图，`fal-ai/flux/dev` 用 `image_size`，Kling 用 `aspect_ratio` + `duration`；
Replicate 上 `flux-kontext` 用 `input_image`，`nano-banana` 用 `image_input`。

后端不可能穷举成千上万个模型，所以采用两条通道：

| 通道 | 用途 |
|---|---|
| **通用字段映射** | `prompt`、张数、宽高比、时长、参考素材——这些几乎所有模型都认 |
| **模型名后的 query** | 该模型特有的参数，`?key=value` 追加，**优先级最高，可覆盖通用映射** |

模型名示例：

```
fal-ai/flux/dev?image_size=landscape_16_9&num_inference_steps=28
fal-ai/kling-video/v2.1/master/image-to-video?duration=10&negative_prompt=blur
black-forest-labs/flux-dev?num_inference_steps=28&guidance=3.5
kwaivgi/kling-v1.6-standard?image_field=start_image&duration=10
```

> 参数会做类型推断：`28` → 数字，`true/false` → 布尔。需要精确类型时用
> `?params={"num_inference_steps":28,"enable_safety":false}`（值是 JSON，需 URL 编码）。

---

## 二、建渠道

「设置 → 模型渠道 → 新增」，协议选 **Fal.ai** 或 **Replicate**，填 API Key 后保存。

| 协议 | 默认地址 | 鉴权头 | 取 Key |
|---|---|---|---|
| Fal.ai | `https://queue.fal.run` | `Authorization: Key <key>` | https://fal.ai/dashboard/keys |
| Replicate | `https://api.replicate.com/v1` | `Authorization: Bearer <key>` | https://replicate.com/account/api-tokens |

- 两个平台都**不提供统一的模型列表接口**，点"获取模型"会提示手动填写是正常的，
  在"输入模型名称"里直接键入模型路径即可。
- Fal 的 Key 直接粘贴原始值就行，不需要自己加 `Key ` 前缀。

---

## 三、模型名格式

### Fal.ai

```
{模型路径}
{模型路径}?{参数}
```

模型路径就是 fal 模型页 URL 里 `fal.run/` 之后的部分，例如
`fal.run/fal-ai/flux/dev` → 填 `fal-ai/flux/dev`。

### Replicate

```
{owner}/{name}                     官方模型
{owner}/{name}:{version}          社区模型（Replicate 官方写法）
{owner}/{name}?version={version}  同上，另一种写法
```

**官方模型与社区模型的提交地址不同**，画布会自动判断：

| 形式 | 提交地址 |
|---|---|
| 有 version | `POST /v1/predictions`，报文含 `version` |
| 无 version | `POST /v1/models/{owner}/{name}/predictions` |

> Replicate 文档明确：`/models/{owner}/{name}/predictions` **只适用于官方模型**，
> 其他模型必须带 version。所以社区模型请务必写上 `:version`。
> 页面 URL 上形如 `replicate.com/owner/name/versions/abc123...` 的那串就是 version。

---

## 四、通用字段映射

| 画布字段 | Fal.ai | Replicate | 说明 |
|---|---|---|---|
| `prompt` | `prompt` | `input.prompt` | 一定映射 |
| 生成张数 `n`（>1 时） | `num_images` | `input.num_outputs` | 数字 |
| 视频时长 `seconds` | `duration`（**字符串**） | `input.duration`（**数字**） | 类型差异由后端处理 |
| 视频尺寸 | `aspect_ratio`（如 `16:9`） | `input.aspect_ratio` | 由 `1280x720` 折算成 `16:9` |
| 参考图 | `image_url` / `image_urls` | `input.image` / `input.image_input` | 单图用前者，多图用后者 |
| 参考视频 | `video_url` / `video_urls` | `input.video` / `input.video_input` | 仅视频接口 |
| 参考音频 | **不支持，会直接报错** | **不支持，会直接报错** | 见下 |

不映射的字段（`quality`、`resolution_name`、`preset`、`stream`、`response_format`、
`video_generate_audio` 等）会被丢弃——因为它们在两个平台上没有统一名字。
需要就用 query 手动传，例如 `?resolution=1080p`。

### 字段名不对怎么办

参考素材的字段名可以用 query 覆盖，覆盖键本身**不会**发给上游：

| 覆盖键 | 默认值 | 举例 |
|---|---|---|
| `image_field` | fal `image_url` / replicate `image` | `?image_field=input_image` |
| `image_field_plural` | fal `image_urls` / replicate `image_input` | `?image_field_plural=images` |
| `video_field` | fal `video_url` / replicate `video` | `?video_field=video_url` |
| `video_field_plural` | fal `video_urls` / replicate `video_input` | |

---

## 五、参考素材怎么传

平台拿不到你浏览器里的本地图片，所以画布的本地参考图有两种处理方式：

| 平台 | 方式 |
|---|---|
| **Fal.ai** | 后端把本地图**内联成 data URI** 直接塞进请求体（fal 接受 data URI，无大小限制） |
| **Replicate** | 后端先调用 Replicate 的 `POST /v1/files` 上传，拿到地址再作为输入（官方文档：data URI 只建议用于 1MB 以内的文件） |

两边都用到画布已有的"参考素材占位符 → 替换"机制，未改上游行为。
**参考音频两个平台都不支持**，附了音频会得到明确报错而不是静默丢弃。

---

## 六、轮询与结果

| 平台 | 提交 | 取结果 |
|---|---|---|
| Fal.ai | `POST {base}/{模型路径}` → `request_id` | `GET {base}/{模型路径}/requests/{id}/response`，**未完成返回 202**，完成返回 200 + 模型输出 |
| Replicate | `POST {base}/predictions` 或 `/models/{o}/{n}/predictions` | `GET {base}/predictions/{id}`，读 `status`，完成时从 `output` 取地址 |

产物地址的读取是**白名单**的：只认 `images/image`、`video/videos/video_url`、`audio/audio_url`
这些输出字段。Fal 提交响应里的 `status_url` / `response_url` 是合法 URL，但不会被误当成产物。

---

## 七、排错

| 现象 | 原因 |
|---|---|
| `Fal 任务缺少请求 ID 或模型 ID` | 模型名没填，或填了纯参数 |
| `Fal 图片编辑需要至少一张参考图` | 该模型是图生图，但画布里没放参考图 |
| `Replicate 模型名需为 owner/name 或 owner/name:version` | 模型名格式不对 |
| `Replicate 图片编辑需要至少一张参考图` | 同上 |
| `Fal 渠道暂不支持参考音频` | 附了音频参考 |
| `422 Unprocessable Entity` / 模型报字段非法 | 该模型的输入字段名和默认值不同，用 `?key=value` 或 `?image_field=` 修正 |
| 浏览器控制台出现 CORS 报错 | 浏览器直连平台被拦。两者官方 JS SDK 都支持浏览器直连，正常网络下不应出现 |
| `401 / 403` | Key 填错。Fal 请粘贴原始 key（画布会补 `Key ` 前缀），Replicate 用 `r8_` 开头的 token |

**验证渠道是否接通的最快方式**：在画布用 `fal-ai/flux/dev`（或任一官方 Replicate 模型）
跑一次纯文生图，只填 prompt。这类调用最小、最容易成功。
