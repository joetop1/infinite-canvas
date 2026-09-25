# 改造台账

## v0.8.0-custom.2 — Fal Veo 3 Fast 参考图拦截

- Fal 的 `fal-ai/veo3/fast` 是文生视频端点；此前能力识别只匹配路径以 `/text-to-video` 结尾的模型 ID，因此连带参考图一起发送，触发上游 405。
- 将 `fal-ai/veo3` 与 `fal-ai/veo3/fast` 纳入文生视频识别。画布已连接图片时，模型会从可选项隐藏；旧节点会被拦截并提示更换支持图片的模型，避免继续向该端点发起不兼容请求。
- 请求中的 `fal-ai/veo3/fast`、6 个 `image_urls` 和 405 与该能力不匹配相符；服务端只返回了通用 405，无法仅凭该错误证明唯一根因。
- 未运行构建或测试（遵循仓库 `AGENTS.md`）。

记录本仓库相对上游 [tigerowo/infinite-canvas](https://github.com/tigerowo/infinite-canvas) 的全部改动。
上游更新合并时，先看本文件即可知道"哪些是自有改动"。

验收口径见 `README.md` 第一节：`git diff --numstat upstream/main..custom` 的删除行，只允许是与新增行成对的行内改写。真正删掉上游代码即为违规。

截至 `v0.7.1-custom.5`：**上游文件共 13 个被改过**，其中 5 个只有新增行，8 个含"行内改写"性质的删除行；后续版本的新增改动见下方对应记录——

| 上游文件 | 新增 | 删除 | 性质 |
|---|---|---|---|
| `README.md` | 6 | 0 | 纯新增 |
| `web/src/services/api/video.ts` | 23 | 4 | 4 行行内改写 |
| `web/src/services/api/direct-ai.ts` | 12 | 4 | 4 行行内改写 |
| `web/src/services/api/protocols/types.ts` | 3 | 1 | 1 行行内改写 |
| `web/src/services/api/protocols/direct-registry.ts` | 4 | 0 | 纯新增 |
| `web/src/services/api/protocols/direct-registry.test.ts` | 102 | 1 | 1 行行内改写 |
| `web/src/lib/model-channel.ts` | 2 | 0 | 纯新增 |
| `web/src/lib/model-channel.test.ts` | 9 | 1 | 1 行行内改写 |
| `handler/ai.go` | 17 | 4 | 4 行行内改写（2 行 gofmt 字段对齐、2 处请求上下文/错误透传） |
| `handler/model_protocol.go` | 46 | 0 | 纯新增 |
| `handler/model_protocol_direct_test.go` | 84 | 0 | 纯新增 |
| `handler/video_task.go` | 4 | 2 | 2 行行内改写（错误透传与请求上下文） |
| `service/model_protocol.go` | 61 | 8 | 8 行行内改写（7 行是 gofmt 对齐，1 行是列表加项） |

无一处是净删除。删除行分两类，都可逐行对照：

- **gofmt 对齐**（共 9 行）：`service/model_protocol.go` 7 行——新增 `fal` / `replicate` 两个常量后重新对齐 `=` 号，原 7 个常量名一字未改；`handler/ai.go` 2 行——结构体新增 `Detail json.RawMessage` 字段后重新对齐 `Msg` / `Message` 的字段类型，字段名与 tag 一字未改。
- **其他行内改写**（其余 16 行）：包括视频代理与图片代理的请求上下文、Fal/Replicate 参数错误透传，以及 `modelProtocolIDs`、`pollURL`、鉴权、202 兼容、协议白名单等挂载点。每一处都在同一位置有新版本顶上。

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
| `handler/ai.go` | `readUpstreamAIErrorMessage()`、`proxyAIRequest()` | 结构体加 `Detail` 字段（行内改写 2 行）；追加上游错误解析、参数错误透传和请求上下文（新增代码均有 `[CUSTOM]` 标记），解析逻辑全在新文件 `handler/upstream_error_message.go` |
| `handler/model_protocol.go` | `builtinAIProtocols` 表 | 追加 `fal` / `replicate` 两个适配器（纯新增 46 行，插在 `model:agnes` 之前） |
| `handler/video_task.go` | `proxyAIVideoTaskRequest()` | Fal/Replicate 参数错误透传；上游视频请求继承用户请求上下文（2 行行内改写） |
| `service/model_protocol.go` | 常量块与 `init()` 注册表 | 追加 2 个协议常量、2 个 `modelProtocolIDs` 项、2 段注册逻辑（含 3 个新函数 `IsFalChannel` / `IsReplicateChannel` / `FalAuthorizationHeader`） |
| `service/model_protocol.go` | `modelDiscoveryRules` | 追加 `fal` / `replicate` 两条规则（2 行代码 + 2 行注释）。**缺少它们会让"拉取模型列表"回落到 OpenAI 的 `/models`** |
| `service/model_protocol.go` | `modelConfigTestRules` | 追加 `fal` / `replicate` 两条规则（2 行代码 + 1 行注释），否则"测试渠道"会去打 `chat/completions` |
| 三个 `*_test.go` | `handler/model_protocol_direct_test.go`、`web/src/lib/model-channel.test.ts`、`web/src/services/api/protocols/direct-registry.test.ts` | 断言表追加用例；后两者各有 1 行行内改写（协议白名单加项、`fal` 队列地址断言） |

**新增文件共 17 个**（上游不存在，零冲突）——它们不构成冲突面，可直接 `git merge`：

| 位置 | 文件 |
|---|---|
| 仓库根 | `docker-compose.custom.yml` |
| `handler/`（放上游目录是为了复用同包非导出符号，见 `README.md` 第四节例外） | `fal_request.go`、`replicate_request.go`、`direct_model_spec.go`、`direct_queue.go`、`upstream_error_message.go`、`model_protocol_proxy_test.go`、`model_protocol_queue_test.go` |
| `service/` | `custom_direct_models.go`、`custom_direct_models_test.go` |
| `web/src/services/api/protocols/` | `fal.ts`、`replicate.ts` |
| `docs/custom/`、`scripts/` | `README.md`、`CHANGELOG-custom.md`、`DEPLOY.md`、`FAL-REPLICATE.md`、`sync-upstream.sh` |

核对命令（改动 vs 新增一眼可分）：

```bash
for f in $(git diff --numstat upstream/main | awk '{print $3}'); do
  printf '%-58s ' "$f"
  git cat-file -e upstream/main:"$f" 2>/dev/null && echo '上游已有 → 改动' || echo '★ 新增文件'
done
```

## 变更记录

### v0.8.0-custom.1 — 同步上游 0.8.0 并保留本仓库改造

- **上游更新**：合入 `v0.8.0`，加入 RunningHub 工作流与本地 ComfyUI Bridge 等上游功能；根目录 `VERSION` 更新为 `v0.8.0`。
- **冲突处理**：保留上游新增的工作流模型选项与协议 `kie`，同时保留 Fal / Replicate 渠道，并让视频模型选择器同时支持工作流与“图片参考时隐藏 Fal 文生视频”规则。
- **验证状态**：自有改动差异检查通过，未发现未解决冲突；未运行构建或测试。RunningHub / ComfyUI Bridge 与 Fal / Replicate 仍需人工验收。

### v0.7.1-custom.6 — 模型目录搜索与视频参考图校验

- **模型列表**：Fal / Replicate 原先只提供 24 / 25 个内置常用模型。管理页模型选择器现在支持按关键词查询平台公开目录；Replicate 每次最多返回 50 个结果，Fal 每次最多取 100 个。搜索结果默认不勾选，用户确认后才写入渠道配置。
- **避免错误的视频调用**：画布视频节点连接图片参考时，隐藏 Fal 文生视频模型；已有节点若仍配置该模型，会提示并禁用生成按钮，前端请求层也会在发请求前拦截。
- **文档**：修正 Fal / Replicate 目录检索方式的过时说明；详情见 `FAL-REPLICATE.md`。
- **验证状态**：差异检查通过；依项目约定未运行构建和测试。真实 Fal / Replicate 搜索与模型调用、参考图拦截仍需人工验收。

### v0.7.1-custom.5 — 修复账号渠道下的「AI 接口请求失败：400」

- **上游挂载点**：`handler/ai.go`（**新增的第 12 个**，17 增 / 4 改）；`handler/video_task.go` 增加 4 行 / 2 处行内改写
- **新增文件**：`handler/upstream_error_message.go`、`handler/direct_queue.go`、
  `handler/model_protocol_proxy_test.go`、`handler/model_protocol_queue_test.go`
- **改写文件**：`handler/fal_request.go`、`handler/replicate_request.go`、`handler/direct_model_spec.go`、
  `handler/model_protocol.go`、`web/src/services/api/protocols/fal.ts`、
  `handler/video_task.go`、`web/src/services/api/protocols/direct-registry.test.ts`
- **症状**：**账号渠道**（登录后、渠道建在服务端）用 Fal / Replicate 生成，只得到一句
  `AI 接口请求失败：400`。本地直连（渠道存在浏览器里）却正常。

- **一句话里藏着三个独立 bug**，逐个定位。这三处**都不是同一个原因**，
  修掉任意一处另外两处仍会让它失败：

  **① mode 门 —— 账号渠道的报文根本没被转译。**

  `prepareFalRequest` / `prepareReplicateRequest` 开头写的是：

  ```go
  if input.mode != aiProtocolDirectRequest { return nil, nil }
  ```

  但这个 `prepare` 钩子**三条链路都会调用**：

  | `input.mode` | 链路 |
  |---|---|
  | `aiProtocolDirectRequest` | 本地直连的"参数转译接口"（`/api/ai/direct-request`） |
  | `aiProtocolProxyRequest` | **账号渠道走画布后端代理** ← 被门挡住的正是这条 |
  | `aiProtocolVideoRequest` | 视频创作台建任务 |

  于是登录后走代理时函数直接空转返回，**画布形态的报文（`model` / `size` / `n`）原样发给了平台**——
  平台当然拒收，这就是那个 `400`。

  **改法**：去掉 mode 门，改判「这个渠道是不是 Fal / Replicate」（`isFalEndpoint` / `isReplicateEndpoint`）。
  判据从"谁在调用我"换成"我在处理谁"，与调用链路无关，三条链路自动全部生效。

  **② 错误体不认 —— 平台明明说了原因，被丢掉了。**

  `readUpstreamAIErrorMessage` 只认 `error.message` / `msg` / `message`，
  而 Fal 与 Replicate 都是 FastAPI，报错体形如 `{"detail": ...}`（字符串，
  或 `[{"msg": ..., "loc": ..., "type": ...}]` 列表）。读不出来就只剩状态码。

  **改法**：结构体加一个 `Detail json.RawMessage` 字段（2 行行内改写，gofmt 顺带对齐了
  `Msg` / `Message` 的类型列），其后接两处 `[CUSTOM]` 调用；**解析逻辑全部放进新文件
  `handler/upstream_error_message.go`**，上游函数里只留两行调用。用户此后看到的是平台原文，
  例如 `missing: body.prompt`。

  **③ 队列路径多了一段 —— Fal 轮询 405。**

  Fal 的队列按**应用**划分，不按模型。`fal-ai/flux/dev` 的队列是 `fal-ai/flux`。
  用完整模型路径去拼查询地址会多出一段。curl 实测对照：

  | 地址 | 结果 |
  |---|---|
  | `.../fal-ai/flux/requests/{id}/status` | 404 `{"status":"NOT_FOUND"}` —— **路由存在**，只是这个 id 不在该队列 |
  | `.../fal-ai/flux/dev/requests/{id}/status` | **405** —— 多了一段 |

  `fal-ai/kling-video/v2.1/master/text-to-video` 同理只取到 `fal-ai/kling-video`。

  **改法**：新增 `falQueuePath()`（Go 与 TS 各一份），只取前两段。**提交地址仍用完整模型路径**，
  这一点不变。

- **顺带补齐的一条链路：代理模式下后端要替用户把队列跑完。**
  参照上游既有的 `pollAPIMartImageTask` / `copyKIEVideoResponse` 写法：
  图片接口在同一个请求里跑完队列再返回图；视频接口把上游 ID 存成画布任务 ID，之后由画布按任务轮询。
  共用的等待与取址逻辑抽进新文件 `handler/direct_queue.go`（固定 2 秒间隔、上限 300 次，
  并 `select` 监听 `request.Context().Done()`，用户离开后不会继续空转）。
  上游自己返回的查询地址（Fal 的 `status_url` / `response_url`）会被优先使用，
  但**只接受与渠道同源的地址**——否则渠道可以被配置成把请求转去任意第三方。
  产物地址的读取走**白名单字段**，Fal 的 `status_url` / Replicate 的 `urls.get` 这类
  "合法但不是产物"的 URL 不会被误取。

- **账号渠道下的参考素材与本地直连形态不同**：有参考图时画布发的是 **multipart**
  （图片在文件字段里，另有 `_canvas_*` 元字段），本地直连走的则是"占位符 → 浏览器替换"。
  新增 `decodeDirectRequestBody()` 作统一入口：非 multipart 走原有解析；multipart 请求体上限
  64MB，字段名跳过 `_canvas_*`、同名字段聚成数组、文件字段转成
  `data:<mime>;base64,...`（MIME 缺失或为 `application/octet-stream` 时按扩展名补）。单个文件上限 16MB。

- **复核补齐**：代理与视频创建请求现在继承客户端取消上下文；渠道转译错误会显示给用户；
  自上游返回的队列地址要求 scheme 与 host 都匹配；Replicate 的 `aborted` 预测按失败处理。

- **过程中被自家新测试抓出的一个 bug**：`readDirectReferences` 原先**只认**
  `direct-reference.invalid` 这种占位符，而账号渠道传进来的是 data URI 或服务端地址，
  于是报「Fal 图片编辑需要至少一张参考图」。已放宽为「任意 `data:` 前缀或 http(s) 地址」。

- **反向验证**（先让测试失败，再让它通过，确认测试真的能抓住 bug）：

  | 故意制造的错误 | 立刻失败的用例 |
  |---|---|
  | 把 `falQueuePath` 改回完整模型路径 | `TestFalQueueImageProxyFlow/queue_path_drops_model_sub_path`、`TestFalVideoProxyFlow`（并准确报出多出的那一段） |
  | 把 mode 门加回去 | 转译用例失败（`provider` 为空），确认门确实拦住了代理链路 |
  | 移除 `modelDiscoveryRules` 的两条规则（custom.4 时） | `TestCustomDirectModelDiscoveryDoesNotFallBackToOpenAI` |

- **独立复核**：Go 1.27.1 下 `go test ./...`、`go vet ./...`、`go build ./...` 均通过；
  `bun test` **23 pass / 0 fail**；`git diff --check` 通过。`gofmt -d` 只剩
  `handler/model_protocol.go` 中 AutoDL 字段和 `handler/video_task.go` 中既有 `ClientTaskID` 的对齐差异，
  Fal/Replicate 新增代码已格式化。TypeScript 检查仍报错在未改动的 `canvas-client-page.tsx`。
- **原会话记录**：曾报告 `bun run build` 编译与静态页生成成功（20/20）；本次没有重跑前端生产构建。

- **仍需人工验收**：宝塔把 `image` 改为 `ghcr.io/joetop1/infinite-canvas:v0.7.1-custom.5`
  并重启后，用 `fal-ai/flux/dev` 跑一次纯文生图。详见 `FAL-REPLICATE.md` 第七节。

- **一条环境备注**：`bun run build` 结尾的临时文件清理会被宿主 WorkBuddy 的删除护栏拦下
  （`SAFE_DELETE_BULK_CONFIRM_REQUIRED`），**编译本身是成功的**；清理 `.next` 后重跑即可。

### v0.7.1-custom.4 — 修复 Fal/Replicate 渠道「读取模型失败：404」

- **文件**：`service/model_protocol.go`（上游）、新增 `service/custom_direct_models.go` 与 `service/custom_direct_models_test.go`
- **症状**：建好 Fal.ai 渠道后，点「选择模型 → 拉取模型列表」弹 `读取模型失败：404`，列表为空，渠道配不上模型。

- **根因**：协议**注册了、但没挂进规则链**。
  `modelProtocolRegistry` 里有 `fal` / `replicate` 两个适配器，`models` 字段也是对的，
  但 `fetchAdminChannelModels()` 走的是 `matchModelProtocol(modelDiscoveryRules, channel, "")`，
  而 `modelDiscoveryRules` 里没有这两条。匹配失败后 `matchModelProtocol` **回落到 OpenAI 适配器**，
  于是去请求 `{baseUrl}/models`——对 Fal 就是 `https://queue.fal.run/models`，404。
  `modelConfigTestRules` 漏得一样，会让「测试渠道」拿 `chat/completions` 去打这两个平台。

  这个疏漏之所以没被拦住：此前的测试只覆盖报文转译（`handler/`）与前端适配器，
  而"协议是否登记进三条规则链"没有任何断言。

- **改法**：
  1. `modelDiscoveryRules` 与 `modelConfigTestRules` 各补 `fal` / `replicate`，各 2 行；
  2. `models` 由"返回错误提示"改为返回内置清单（新增 `service/custom_direct_models.go`）。

- **为什么用内置清单而不是实时拉取**（均实测确认）：
  - `api.fal.ai/v1/models` 可匿名读取，但**不支持 category / q / search 过滤**，只有
    `limit`（上限 200）+ `cursor`，模型总量以千计——实测翻到第 14 页、1400 条时仍未结束。
    整份拉取既慢，几千项的下拉框也无法使用。
  - Replicate 的 `GET /v1/models` 需要鉴权（无鉴权返回 401），且只返回**当前账号自建**的模型，
    拿不到公开目录。
  - 因此沿用上游 `kieMarketModels()` / `MiMoModels()` 的内置清单做法。

- **清单可信度**：不靠印象拼名字，逐个做可达性探测——Fal 用 `fal.ai/models/{endpoint_id}`、
  Replicate 用 `replicate.com/{owner}/{name}`，并用**故意写错的名字做反向对照**（确实返回 404，
  证明探测有效）。据此剔除了 `kwaivgi/kling-v1.6-standard`（404，正确名字是 `kwaivgi/kling-v2.1`）、
  `luma/ray`（404）等失效项。最终 Fal 24 个、Replicate 25 个。

- **新增回归测试**（`service/custom_direct_models_test.go`）：
  - `TestCustomDirectModelDiscoveryDoesNotFallBackToOpenAI` —— 把上游 HTTP 客户端换成
    "一旦被调用就报错"的桩。清单是静态的，只要触发网络就说明发生了回落。
    **已反向验证**：临时移除那两条规则后测试立即失败，打印
    「模型发现发起了网络请求，说明回落到 OpenAI 的 /models 了」；恢复后通过。
  - `TestCustomDirectChannelConfigTestDoesNotCallOpenAI` —— 同一手法守住 `modelConfigTestRules`。
  - `TestCustomDirectModelListShape` —— 断言清单非空、无重复、Fal 形如 `fal-ai/xxx`、
    Replicate 形如 `owner/name`（内置项不带 `:version`）。名字最终会原样拼进请求地址，
    写错一个字符就等于让用户白跑一次生成。

- **验证**：`go vet ./...` 无输出、`go test ./...` 全绿（service 包新增 3 例）。

### v0.7.1-custom.3 — 接入 Fal.ai / Replicate 渠道

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
  - **模型列表发现**：初版把 `models` 写成"返回一句提示文案"，但漏了把它挂进模型发现规则链，
    导致读取模型时 404 —— 已在 `v0.7.1-custom.4` 修正为内置清单。
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
