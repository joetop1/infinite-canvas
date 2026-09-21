# 改造台账

记录本仓库相对上游 [tigerowo/infinite-canvas](https://github.com/tigerowo/infinite-canvas) 的全部改动。
上游更新合并时，先看本文件即可知道"哪些是自有改动"。

验收口径见 `README.md` 第一节：`git diff --numstat upstream/main..custom` 的删除行，只允许是与新增行成对的行内改写。真正删掉上游代码即为违规。

当前状态（`3 新增 / 1 删除`，删除的那 1 行是 `video.ts` 中被改写的 `if` 语句，与新版本成对）。

## 挂载点清单（改动上游文件的全部位置）

| 上游文件 | 位置 | 改动性质 |
|---|---|---|
| `web/src/services/api/video.ts` | `cacheProtectedVideo()` | 追加 3 行（含 1 行注释），无删除 |

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

## 有意未改的上游文件

- `docs/progress/todo.md`、`docs/progress/pending-test.md`：项目约定要求每次变更同步更新，但这两个文件上游高频改动，属于必然冲突点。自有变更统一记在本文件，不改它们。
- `.github/workflows/`：未删除、未修改（首次推送时因令牌缺 `workflow` 权限被 GitHub 拒绝过，解法是补权限而非删文件）。
