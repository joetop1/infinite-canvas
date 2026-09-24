# 改造台账

记录本仓库相对上游 [tigerowo/infinite-canvas](https://github.com/tigerowo/infinite-canvas) 的全部改动。
上游更新合并时，先看本文件即可知道"哪些是自有改动"。

验收口径见 `README.md` 第一节：`git diff --numstat upstream/main..custom` 的删除行，只允许是与新增行成对的行内改写。真正删掉上游代码即为违规。

当前状态：**上游文件共 11 个被改过**，其中 5 个只有新增行，6 个含"行内改写"性质的删除行——

| 上游文件 | 新增 | 删除 | 性质 |
|---|---|---|---|
| `README.md` | 6 | 0 | 纯新增 |
| `web/src/services/api/video.ts` | 23 | 4 | 4 行行内改写 |
| `web/src/services/api/direct-ai.ts` | 12 | 4 | 4 行行内改写 |
| `web/src/services/api/protocols/types.ts` | 3 | 1 | 1 行行内改写 |
| `web/src/services/api/protocols/direct-registry.ts` | 4 | 0 | 纯新增 |
| `web/src/services/api/protocols/direct-registry.test.ts` | 85 | 1 | 1 行行内改写 |
| `web/src/lib/model-channel.ts` | 2 | 0 | 纯新增 |
| `web/src/lib/model-channel.test.ts` | 9 | 1 | 1 行行内改写 |
| `handler/model_protocol.go` | 36 | 0 | 纯新增 |
| `handler/model_protocol_direct_test.go` | 84 | 0 | 纯新增 |
| `service/model_protocol.go` | 54 | 8 | 8 行行内改写（7 行是 gofmt 对齐，1 行是列表加项） |

无一处是净删除。删除行分两类，都可逐行对照：

- **go 常量块对齐**（`service/model_protocol.go` 7 行）：新增 `fal` / `replicate` 两个常量后 `gofmt` 重新对齐 `=` 号，原 7 个常量名一字未改。
- **单行扩写**（其余 12 行）：`modelProtocolIDs` 列表加项、`pollURL` 签名加参数、`Authorization` 头改走新的 `directAuthorization()`、`response.ok` 判断兼容 202、`ark` 内联条件加上 `fal`、测试里的协议白名单加项。每一处都在同一位置有新版本顶上。

## 挂载点清单（改动上游文件的全部位置）

| 上游文件 | 位置 | 改动性质 |
|---|---|---|
| `README.md` | 标题与徽章之间 | 插入 6 行 AGPL §5(a) 修改声明（协议要求"显著"，故不能放文件末尾） |
| `web/src/services/api/video.ts` | `cacheProtectedVideo()` | 追加 3 行（含 1 行注释），行内改写 1 行 |
| `web/src/services/api/video.ts` | `cacheProtectedVideo()` 之下 | 新增独立函数 `fetchVideoContent()`（20 行，含注释与空行），行内改写 2 行 |
| `web/src/services/api/direct-ai.ts` | `uploadAndReplaceReferences()` | 行内改写 1 行（内联条件加入 `provider === "fal"`），追加 1 行注释 |
| `web/src/services/api/direct-ai.ts` | `requestDirectJSON()` | 行内改写 1 行（`Authorization` 改用 `directAuthorization()`），追加 3 行（202 容忍） |
| `web/src/services/api/direct-ai.ts` | `requestDirectJSON()` 之下 | 新增独立函数 `directAuthorization()`（4 行） |
| `web/src/services/api/direct-ai.ts` | `directPollURL()` | 行内改写 1 行（把模型名传给 `pollURL`） |
| `web/src/services/api/protocols/types.ts` | `DirectProtocolAdapter` | 追加 `authorization?` 字段与注释；`pollURL` 签名增加可选 `model` 参数（行内改写 1 行） |
| `web/src/services/api/protocols/direct-registry.ts` | 文件头 import 与注册表 | 追加 `fal` / `replicate` 两条（纯新增 4 行） |
| `web/src/lib/model-channel.ts` | `modelChannelProtocols` 数组 | 追加 `fal` / `replicate` 两条协议项（纯新增 2 行） |
| `handler/model_protocol.go` | `builtinAIProtocols` 表 | 追加 `fal` / `replicate` 两个适配器（纯新增 36 行，插在 `model:agnes` 之前） |
| `service/model_protocol.go` | 常量块与 `init()` 注册表 | 追加 2 个协议常量、2 个 `modelProtocolIDs` 项、2 段注册逻辑（含 3 个新函数 `IsFalChannel` / `IsReplicateChannel` / `FalAuthorizationHeader`） |
| 两个 `*_test.go` | 文件内追加用例 | 纯新增，未改任何既有断言（`direct-registry.test.ts` 有 1 行行内改写：协议白名单加项） |

新增文件（上游不存在，零冲突）：`web/src/services/api/protocols/fal.ts`、
`web/src/services/api/protocols/replicate.ts`、`handler/fal_request.go`、
`handler/replicate_request.go`、`handler/direct_model_spec.go`、`docs/custom/FAL-REPLICATE.md`。

## 变更记录

### 接入 Fal.ai / Replicate 渠道

- **新增文件**：`web/src/services/api/protocols/fal.ts`、`web/src/services/api/protocols/replicate.ts`、
  `handler/fal_request.go`、`handler/replicate_request.go`、`handler/direct_model_spec.go`、`docs/custom/FAL-REPLICATE.md`
- **目标**：让画布能直接使用 Fal.ai 与 Replicate（用户自带 Key、浏览器直连平台），覆盖文生图、图生图、文/图生视频。

- **为什么走"直连协议"这条路**：上游已有一套成熟的直连协议插槽——前端 `protocols/*.ts` 负责鉴权头、任务 ID、轮询与产物解析，
  后端 `builtinAIProtocols` 负责提交地址与报文转译。Fal/Replicate 正好属于这一类（第三方平台 + 自带 Key），
  因此**顺着插槽加两个协议**即可，不需要动 AI 代理、渠道管理、画布节点等任何既有逻辑。

- **核心设计：模型专属参数走"模型名后的 query"**。这是本次最关键的一个判断。
  两个平台每个模型的输入 schema 都不同（`flux/dev` 用 `image_size`、Kling 用 `aspect_ratio`+`duration`、
  Replicate 上 `input_image` / `image_input` / `start_image` 三种写法并存），后端无法穷举。所以：

  - 后端只映射**通用字段**：`prompt`、张数、宽高比、时长、参考素材字段；
  - 其余参数通过模型名追加，如 `fal-ai/flux/dev?image_size=landscape_16_9&num_inference_steps=28`，
    支持类型推断（`28`→数字、`true`→布尔）与 `?params={...}` 精确 JSON 两种写法；
  - **query 参数的优先级最高**，可覆盖通用映射，用户始终有最终解释权；
  - 参考素材字段名可用 `?image_field=` / `?image_field_plural=` 等**只作用于本地映射、不发给上游**的键覆盖。

  这样既不用为每个模型写适配器，也不会因为猜错字段名而没有出路。

- **平台差异（都在实现里处理掉了）**：

  | 差异点 | Fal.ai | Replicate |
  |---|---|---|
  | 鉴权头 | `Authorization: Key <key>` | `Authorization: Bearer <key>` |
  | 提交地址 | `POST {base}/{模型路径}` | 有 version → `/predictions`；无 → `/models/{o}/{n}/predictions` |
  | 取结果 | `GET {base}/{模型路径}/requests/{id}/response` | `GET {base}/predictions/{id}` |
  | 未完成的表示 | **HTTP 202 + 空体** | 200 + `status: starting/processing` |
  | 结果结构 | 模型输出本体（`images[]` / `video` / `audio_url`） | `status` + `output` |
  | 时长类型 | 字符串 `"5"` | 数字 `5`（Replicate 严格校验类型） |
  | 本地参考素材 | 内联 data URI（fal 接受，无大小限制） | 先调 `POST /v1/files` 上传再传地址（官方文档：data URI 仅建议 1MB 以内） |

- **为此在上游代码里加了 3 个扩展点**（都是最小改动，见"挂载点清单"）：

  1. `DirectProtocolAdapter.authorization?(apiKey)` —— Fal 的 `Key ` 前缀需要自定义，
     原有的 `rawAuthorization` 布尔开关只能表达"原样透传"。新增钩子后 `autodl`（透传）、
     `kie`/`apimart`/`ark`（Bearer）行为完全不变。
  2. `pollURL(baseUrl, taskId, model?)` —— Fal 的结果地址必须包含模型路径，原签名拿不到模型名。
  3. `requestDirectJSON` 容忍 **202** —— 原先 `!response.ok` 一律抛错，会把 Fal 的"仍在排队"当成失败。

- **有意做的取舍**：
  - **不实现模型列表发现**。Fal 没有统一的模型列表接口；Replicate 的 `/v1/models` 虽然存在，
    但未经验证的解析逻辑一旦出错会给出误导性结果。两家都改为返回一句**明确的提示文案**
    （沿用上游 Ark Agent Plan 的既有做法），引导用户在"输入模型名称"里手填。
  - **参考音频直接报错**，不静默丢弃。附了音频却悄悄不生效，比明确报错更难排查。
  - 不映射 `quality` / `resolution_name` / `preset` 等平台间无统一名字的字段，需要就用 query 手动传。

- **验证方式**（均已本地跑通）：
  - `go vet ./...` 无输出；`go test ./...` 全绿（新增 13 个用例：9 组报文 golden + 6 组地址断言 + 1 组鉴权头）。
  - `bun test` 的协议测试 **14 pass / 0 fail**。
  - `bun run build`（Next.js 生产构建，含类型检查）**exit 0**。
  - **真实上游调用仍需人工验证一次**（见 `FAL-REPLICATE.md` 第七节，建议先用 `fal-ai/flux/dev` 跑纯文生图）。

- **过程中修掉的一个自造 bug**：fal 适配器里读取产物地址的 `outputURLs()` 在值为 `undefined` 时
  `outputURLs(asRecord(undefined).url)` 会**无限尾递归**。因为 ESM 是严格模式、JavaScriptCore 实现了尾调用优化，
  它不爆栈而是**把进程挂死**——上一轮 `bun test` 卡了 36 分钟就是这个原因（当时误以为是环境问题）。
  已改为先判断 `"url" in record` 再递归。

### v0.7.1-custom.2 — 取内容路由回退 + 失败可诊断

- **文件**：`web/src/services/api/video.ts`
- **新增函数**：`fetchVideoContent()`
- **背景**：`v0.7.1-custom.1` 加上了"完成任务后去 `/videos/{id}/content` 取视频"的逻辑，但**该路径上线时从未被真实验证过**——最初那次"没有返回视频地址"的报错发生在自建镜像部署之前。也就是说第一版是"照着 new-api 的 OpenAI 兼容说明写的"，存在假设。

- **两个待验证的假设**：

  1. 目标 new-api 实现了 `GET /v1/videos/{id}/content`。new-api 的视频能力历史上走的是任务插件那一套（`/v1/video/generations`），新版才补 OpenAI 风格路由，具体实现取决于部署的版本。
  2. 失败时拿不到任何线索——原实现只抛 `视频内容下载失败：${status}`，无法区分"路由不存在""文件过期""鉴权失败"。

- **改法**：
  - 抽出 `fetchVideoContent()`，先请求新路由 `/videos/{id}/content`；
  - 若返回 **404 / 405**（路由不存在或方法不允许，而非业务错误），**自动回退**旧任务路由 `/video/generations/{id}/content`；
  - 两条都失败时，错误信息里**同时列出两个完整 URL 与各自状态码**，一眼能分清是路由问题还是任务问题。
  - 回退只在协议为 `openai` 时启用（`allowLegacyFallback` 传入 `needsOpenAIContent`），**不影响 88api / grok2api 既有行为**。

- **为什么不直接改成旧路由**：new-api 正在往 OpenAI 标准靠，新路由才是长期正确的那个；回退只是兼容层。保留"新路由优先"能让上游 new-api 升级后无需再改代码。

- **为什么用 404/405 作为回退条件**：这两个码表示"路径本身不存在"，是路由层语义。业务层错误（任务不存在、任务过期）new-api 会返回 4xx/5xx 但带 JSON 报错体，此时回退没有意义，直接抛错更有利于排查。

- **验证方式**：以前端 `tsc --noEmit` 通过（`video.ts` 零错误）。**功能验证仍需真实生成一条视频**——见 `DEPLOY.md` 第 5.2 节。

### 支持 OpenAI 兼容渠道（new-api）的异步视频任务取回

- **文件**：`web/src/services/api/video.ts`
- **函数**：`cacheProtectedVideo()`
- **问题**：基于 new-api 的中转站，其视频任务插件沿用 OpenAI Sora 的异步作业模型，完成响应里**不含任何 url 字段**：

  ```json
  { "id": "task_xxx", "object": "video", "model": "doubao-seedance-2-0-fast-260128",
    "status": "completed", "progress": 100, "created_at": 1789875768, "completed_at": 1789875855 }
  ```

  画布轮询拿到 `completed` 却取不到地址，抛出「视频生成完成但没有返回视频地址」。视频其实已生成，只是没人去取。

- **改法**：在 `cacheProtectedVideo` 的判断里追加一个分支——协议为 `openai`、任务已完成、且没有 url 时，去 `GET /videos/{id}/content` 取回二进制流并存入素材库。与上游既有的 `needs88APIContent` / `needsGrokContent` 分支写法完全一致。

- **为什么不新增协议**：new-api 本就是 OpenAI 兼容站，`openai` 是新增渠道时的默认协议，未知协议也会回退到它。新增协议项需同时改 `web/src/lib/model-channel.ts` 与断言其精确内容的 `web/src/lib/model-channel.test.ts`，代价不值。因此**用户无需改动任何渠道配置**。

- **兼容性**：本地直连（未登录，浏览器直打中转站）与云端代理（走画布后端）两种模式下同一份代码均生效。原因是取内容统一走 `aiApiUrl()` / `aiHeaders()`，且两种模式下 `task.task_id` 都是上游真实任务 ID（云端模式把上游 ID 存在 `task_id`、本地 ID 存在 `id`）。

### AGPL-3.0 §5(a) 修改声明

- **文件**：`README.md`
- **位置**：`<h1>` 标题之后、徽章块之前
- **原因**：上游为 **AGPL-3.0-only**。协议第 5(a) 条要求：基于本程序的作品在分发时，必须**显著地**声明己方修改过，并注明相关日期。本仓库已公开推送，属于分发行为，因此必须履行。
- **为什么必须放顶部**：协议要求"显著"（prominent），放文件末尾不构成显著声明。这是全仓库唯一一处有意违背"改动追加在末尾"规则的例外。
- **为什么选标题与徽章之间**：徽章块（含 version 徽章）是上游的高频改动区，把声明插在它**之前**可与之结构分离，多数情况下 git 能自动合并；只有上游往同一位置插入内容时才会冲突，届时按"两边都保留"处理即可。
- **内容**：声明本仓库为二次开发、给出上游地址与改造起始日期、声明沿用 AGPL-3.0-only，并链接到本台账。

### 二次开发专用部署文件

- **新增文件**：`docker-compose.custom.yml`、`docs/custom/DEPLOY.md`（均为新增，上游无同名文件，零冲突）
- **原因**：上游 `docker-compose.yml` 使用 `image: ghcr.io/tigerowo/infinite-canvas:latest` + `pull_policy: always`，即**拉取上游官方镜像**。直接用它会带来两个问题：二次开发的改动根本不在运行中的镜像里；且每次 `up` 都会拉上游最新版，静默覆盖二次开发成果。
- **做法**：新增与上游同构但改为源码构建的 compose 文件，保持相同的 `container_name` 与 `./data` 数据卷，可原地替换。宿主机端口改为 `${CANVAS_HOST_PORT:-3000}`，便于在多项目共存的主机上避开 3000 冲突。详见 `docs/custom/DEPLOY.md`。
- **注意**：`.dockerignore` 中的 `docs2` 与 `data` 已被上游忽略；`docs/custom/` 不在忽略列表内，会进入构建上下文但不影响产物。

## 有意未改的上游文件

- `docs/progress/todo.md`、`docs/progress/pending-test.md`：项目约定要求每次变更同步更新，但这两个文件上游高频改动，属于必然冲突点。自有变更统一记在本文件，不改它们。
- `.github/workflows/`：未删除、未修改（首次推送时因令牌缺 `workflow` 权限被 GitHub 拒绝过，解法是补权限而非删文件）。
