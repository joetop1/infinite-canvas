# 改造台账

记录本仓库相对上游 [tigerowo/infinite-canvas](https://github.com/tigerowo/infinite-canvas) 的全部改动。
上游更新合并时，先看本文件即可知道"哪些是自有改动"。

验收口径见 `README.md` 第一节：`git diff --numstat upstream/main..custom` 的删除行，只允许是与新增行成对的行内改写。真正删掉上游代码即为违规。

当前状态：**上游文件只有 2 个被改过**——

| 上游文件 | 新增 | 删除 |
|---|---|---|
| `README.md` | 6 | 0 |
| `web/src/services/api/video.ts` | 3 | 1 |

唯一的 1 行删除是 `video.ts` 中被改写的 `if` 语句，与新版本成对，属合规的行内改写。

## 挂载点清单（改动上游文件的全部位置）

| 上游文件 | 位置 | 改动性质 |
|---|---|---|
| `README.md` | 标题与徽章之间 | 插入 6 行 AGPL §5(a) 修改声明（协议要求"显著"，故不能放文件末尾） |
| `web/src/services/api/video.ts` | `cacheProtectedVideo()` | 追加 3 行（含 1 行注释），行内改写 1 行 |

## 变更记录

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
