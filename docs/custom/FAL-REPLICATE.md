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

- Fal / Replicate 渠道在管理页的模型选择器中输入关键词后点“在线搜索”，可查询平台公开目录；结果需手动勾选并确认才会加入渠道。清单外的模型也可以在“输入模型名称”里直接键入路径添加，格式见第三节。
- Fal 的 Key 直接粘贴原始值就行，不需要自己加 `Key ` 前缀。

### 内置模型清单

模型选择器保留一份常用清单作为快速入口；完整公开目录按需在线搜索，不会一次性拉取数千条：

| 平台 | 数量 | 覆盖 |
|---|---|---|
| Fal.ai | 24 | flux 全系、nano-banana、Kling、MiniMax、Wan、Seedance、Veo |
| Replicate | 25 | flux 全系、Imagen、Seedream、GPT-Image、Kling、Veo、Wan |

清单里的名字都做过存在性验证（详见本节末）。在线搜索通过平台目录查询：Fal 使用
`api.fal.ai/v1/models` 的关键词过滤；Replicate 使用 `GET /v1/search`，每次最多返回 50 个模型。
搜索结果不是对某个 API Key 的权限或可用性保证，具体调用能力仍以模型自己的输入 schema 和平台权限为准。

**验证方式**：Fal 用 `fal.ai/models/{endpoint_id}`、Replicate 用 `replicate.com/{owner}/{name}`
探测可达性，并用故意写错的名字做反向对照（确实返回 404），确认探测有效。
模型下线后清单会失效，届时在渠道里手填新名字即可。

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

> ⚠️ **注意：图片尺寸（画布的宽高比选择）不会自动传给上游。**
> Fal 的图片模型习惯用 `image_size` 枚举（`landscape_16_9` / `square_hd` / `portrait_4_3`…），
> Replicate 则各模型不同，猜错字段名会让整个请求 422。所以**图片接口不做尺寸映射**，
> 请用 query 明确指定：`fal-ai/flux/dev?image_size=landscape_16_9`。
> 视频接口的尺寸会被折成 `aspect_ratio`（如 `16:9`），因为这一项在两个平台都比较一致。

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
| **Replicate** | 本地直连时后端先调 `POST /v1/files` 上传，拿到地址再作为输入（官方文档：data URI 只建议用于 1MB 以内的文件） |

- **本地直连**：走画布已有的"参考素材占位符 → 浏览器替换"机制，未改上游行为。
- **账号渠道（登录后）**：画布在有参考图时发的是 **multipart**（图片在文件字段里），
  后端会把文件字段转成 data URI 再拼进平台报文（与上游 APIMart 的处理方式一致）。
  整个 multipart 请求体上限 64MB，单个文件上限 16MB，超过会明确报错。
  > Replicate 官方建议 data URI 在 1MB 以内，账号渠道下超限的图可能被平台拒绝；
  > 此时改用公网图片地址（画布服务端地址）更稳。

**参考音频两个平台都不支持**，附了音频会得到明确报错而不是静默丢弃。

---

## 六、轮询与结果

两个平台都是**队列/异步**平台：提交只回一个任务 ID，产物要再查一次。
画布有两条链路，**两条都已支持**，用户不需要关心走的是哪条：

| 链路 | 何时使用 | 谁负责轮询 |
|---|---|---|
| **浏览器直连** | 本地渠道模式（渠道与 Key 存在浏览器里） | 前端协议适配器（`web/src/services/api/protocols/*.ts`） |
| **画布后端代理** | 账号渠道（登录后，渠道建在服务端） | 画布后端（`handler/fal_request.go`、`handler/replicate_request.go`） |

> 后端代理这条是 `v0.7.1-custom.5` 才补上的。此前两个平台的转译只在"本地参数转译"
> 这一种模式下生效，账号渠道（登录后）走的是后端代理，报文没被转译、队列也没人跟进，
> 表现就是上游 400 / 卡住不动。

| 平台 | 提交 | 取结果 |
|---|---|---|
| Fal.ai | `POST {base}/{模型路径}` → `request_id` + `status_url` / `response_url` | 先 `GET {base}/{owner}/{app}/requests/{id}/status` 等 `COMPLETED`，再取 `.../response` |
| Replicate | `POST {base}/predictions` 或 `/models/{o}/{n}/predictions` → `id` + `urls.get` | `GET {base}/predictions/{id}`，读 `status`，完成时从 `output` 取地址 |

Replicate 的 `failed`、`canceled`、`aborted` 状态会作为失败返回；`aborted` 表示任务开始前已终止。

> ⚠️ **Fal 的队列路径只取模型 ID 的前两段。** fal 的队列按"应用"划分，
> `fal-ai/flux/dev` 的队列是 `fal-ai/flux`，`fal-ai/kling-video/v2.1/master/text-to-video`
> 的队列是 `fal-ai/kling-video`。用完整模型路径请求队列接口会得到 **405**（路由不存在），
> 实测对照：`.../fal-ai/flux/requests/{id}/status` → 404 `{"status":"NOT_FOUND"}`（路由在），
> `.../fal-ai/flux/dev/requests/{id}/status` → 405（多了一段）。
> 提交地址仍用完整模型路径，这一点不变。

产物地址的读取是**白名单**的：只认 `images/image`、`video/videos/video_url`、`audio/audio_url`
这些输出字段。Fal 提交响应里的 `status_url` / `response_url`、Replicate 轮询响应里的
`urls.get` / `urls.cancel` 都是合法 URL，但不会被误当成产物。

代理模式下的排队等待由**画布后端**完成（与上游 APIMart / KIE 的做法一致）：
图片接口在同一个请求里跑完队列，视频接口把上游 ID 存成画布任务 ID，之后由画布按任务轮询。
上游自己返回的查询地址会被优先使用，但只接受**同源**地址（防止渠道被配置成把请求转去别处）。

---

## 七、排错

| 现象 | 原因 |
|---|---|
| `读取模型失败：404` | `v0.7.1-custom.3` 及更早版本的 bug：这两个协议没登记进"模型发现"规则，请求落到了 OpenAI 的 `/models`，实际打的是 `queue.fal.run/models`。升级到 `v0.7.1-custom.4` 及以上即可 |
| `AI 接口请求失败：400` | `v0.7.1-custom.4` 及更早版本的症状：账号渠道下的报文没被转译就直接发给了平台（`400` 就是平台拒绝的），而平台的报错体形如 `{"detail": ...}`，画布读不出来，只剩一句状态码。**升级到 `v0.7.1-custom.5` 后会显示平台原文**（如 `missing: body.prompt`），不再是无信息的状态码 |
| 出现 `detail` / `type: loc` 之类的英文报错 | 这是平台返回的**参数校验错误原文**，现在会如实显示。按提示修正模型名后的 query 即可 |
| `Fal 任务缺少请求 ID 或模型 ID` | 模型名没填，或填了纯参数 |
| `Fal 图片编辑需要至少一张参考图` | 该模型是图生图，但画布里没放参考图 |
| `Fal 任务已完成但没有返回图片地址` | 模型不是图片模型（选错了），或该模型的输出字段不在白名单里 |
| `上游队列任务超时` | 排队/生成超过约 10 分钟。可稍后在创作台看结果，或换更轻的模型 |
| `Replicate 模型名需为 owner/name 或 owner/name:version` | 模型名格式不对 |
| `Replicate 图片编辑需要至少一张参考图` | 同上 |
| `Fal 渠道暂不支持参考音频` | 附了音频参考 |
| `422 Unprocessable Entity` / 模型报字段非法 | 该模型的输入字段名和默认值不同，用 `?key=value` 或 `?image_field=` 修正 |
| 浏览器控制台出现 CORS 报错 | 浏览器直连平台被拦。两者官方 JS SDK 都支持浏览器直连，正常网络下不应出现 |
| `401 / 403` | Key 填错。Fal 请粘贴原始 key（画布会补 `Key ` 前缀），Replicate 用 `r8_` 开头的 token |

**验证渠道是否接通的最快方式**：在画布用 `fal-ai/flux/dev`（或任一官方 Replicate 模型）
跑一次纯文生图，只填 prompt。这类调用最小、最容易成功。
