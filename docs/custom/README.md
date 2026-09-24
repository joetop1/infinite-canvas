# 本仓库改造约定

本目录下的内容均为**本仓库自有改造**，不属于上游项目。核心目标是：**在上游持续更新的前提下，始终保持可合并。**

## 一、铁律：只加不改

- **新增文件永远不会冲突**，冲突只发生在「你改了、上游也改了同一处」。
- 因此所有自有逻辑一律**新建文件**，不往上游的业务文件里写代码。
- 上游文件只允许在**挂载点**做最小改动（见下文），单个文件改动控制在个位数行。
- 优先「在文件末尾追加」；当必须改写已有语句时（例如扩展一个布尔条件），允许行内替换，但需登记到挂载点清单。
- 任何自有改动都必须带 `[CUSTOM]` 注释标记，便于合并时快速识别。

### 验收口径

```bash
git diff --numstat upstream/main..custom
```

输出为 `新增行  删除行  文件`。**删除行只允许是「与某条新增行成对」的行内改写**——即某一行被替换成了新版。若删除行数明显多于被改写的语句数，说明上游代码被真正删除了，必须回滚并重新设计改法。

## 二、远程与分支

| 名称 | 指向 | 用途 |
| --- | --- | --- |
| `upstream` | `tigerowo/infinite-canvas` | 只读同步，**推送通道已封禁**（`NO_PUSH_DISABLED`） |
| `origin` | 本仓库自己的 GitHub 地址 | 提交与推送 |

| 分支 | 职责 |
| --- | --- |
| `main` | **纯上游镜像**，跟踪 `upstream/main`，不提交任何自有改动 |
| `custom` | **所有自有改造**都在这里 |

`main` 之所以保持纯净，是为了随时能拿它当参照物，一眼看清「我到底改了什么」。

## 三、挂载点清单

以下是**全部**允许改动上游文件的位置，由 `docs/custom/CHANGELOG-custom.md` 的统计表逐行核对。
改动时统一加注释标记 `[CUSTOM]`，方便合并冲突时快速识别。

| 上游文件 | 允许的改动 | 规模 |
| --- | --- | --- |
| `README.md` | 标题与徽章之间插入 AGPL §5(a) 修改声明 | 6 行 |
| `router/router.go` | 末尾追加自有模块注册，如 `custom.Register(v1)` | 一行（暂未使用） |
| `main.go` | 初始化自有模块 | 2–3 行（暂未使用） |
| `repository/db.go` | `AutoMigrate(...)` 参数中追加自有 model | 仅新增数据表时（暂未使用） |
| `web/src/constant/navigation-tools.ts` | 数组末尾追加菜单对象 | 暂未使用 |
| `web/src/app/(user)/layout.tsx` | 仅在需要登录态初始化时改动 | 通常无需改动 |
| `web/src/services/api/video.ts` | `cacheProtectedVideo()` 的取回分支 + 新函数 `fetchVideoContent()` | 23 增 / 4 改 |
| `web/src/services/api/direct-ai.ts` | `authorization` 钩子、202 容忍、Fal 参考素材内联、`pollURL` 传模型名 | 12 增 / 4 改 |
| `web/src/services/api/protocols/types.ts` | `DirectProtocolAdapter` 加 `authorization?`、`pollURL` 加 `model?` | 3 增 / 1 改 |
| `web/src/services/api/protocols/direct-registry.ts` | 注册表追加 `fal` / `replicate` | 4 行 |
| `web/src/lib/model-channel.ts` | `modelChannelProtocols` 追加两个协议项 | 2 行 |
| `handler/model_protocol.go` | `builtinAIProtocols` 追加 `fal` / `replicate` 适配器 | 36 行 |
| `service/model_protocol.go` | 2 个协议常量、`modelProtocolIDs` 加项、注册逻辑与 3 个新函数；`modelDiscoveryRules` 与 `modelConfigTestRules` 各补 `fal` / `replicate` 两条 | 61 增 / 8 改 |
| `web/src/lib/model-channel.test.ts`、`web/src/services/api/protocols/direct-registry.test.ts`、`handler/model_protocol_direct_test.go` | 断言表追加用例（不改既有断言） | — |

**除以上文件外，任何上游文件都不应出现自有改动。** 若发现必须新增挂载点，先在本文件登记，再动手。

> 边界口径：**「改上游文件」指改动已有内容**。在上游目录里**新建**文件（如
> `web/src/services/api/protocols/fal.ts`）不算改上游文件——它不会与上游冲突，
> 详见下一节的例外说明。

## 四、自有代码目录约定

| 位置 | 用途 |
| --- | --- |
| `custom/` | 后端自有 Go 代码（独立包，如自有 handler / service / model） |
| `web/src/app/(user)/<自有页面>/` | 前端自有页面（App Router 新增目录即新增路由） |
| `web/src/services/api/<自有模块>.ts` | 前端自有 API 封装 |
| `docs/custom/` | 本约定及相关文档 |
| `scripts/` | 自有脚本 |

### 例外：需要复用上游非导出符号时，新文件必须落在上游目录

Go 的可见性按**包**划分，`custom/` 是另一个包，读不到 `handler` 里的非导出符号
（`aiProtocolRequest`、`directAIUpload`、`directAIReferenceKind` 等）。协议适配器的本质
就是"往上游的协议表里加条目"，必须同包，因此：

| 新文件 | 为什么要放在上游目录 |
| --- | --- |
| `handler/fal_request.go`、`handler/replicate_request.go`、`handler/direct_model_spec.go` | 需要 `package handler` 的非导出类型与函数 |
| `web/src/services/api/protocols/fal.ts`、`replicate.ts` | 需要被 `direct-registry.ts` 以相对路径注册，且要与同目录既有适配器共用 `shared.ts` |

**这类文件全是新增文件，不构成冲突面**，可以直接 `git merge` 过去；
真正需要人工处理冲突的只有上表的挂载点。这也是为什么台账要分"改动行数"与"新增文件"两栏。

## 四之二、功能文档

| 文档 | 内容 |
| --- | --- |
| `docs/custom/CHANGELOG-custom.md` | 改造台账：改了什么、为什么这么改（合并冲突时的权威依据） |
| `docs/custom/DEPLOY.md` | 部署到自有服务器（宝塔面板 + GHCR 镜像） |
| `docs/custom/FAL-REPLICATE.md` | Fal.ai / Replicate 渠道的使用说明与字段映射表 |

## 五、日常同步流程

```bash
# 一键同步上游并合并进 custom
./scripts/sync-upstream.sh
```

手动等价操作：

```bash
git fetch upstream --tags --prune
git checkout main
git merge --ff-only upstream/main      # main 保持快进
git checkout custom
git merge main                         # 把上游更新合进自有分支
```

冲突处理原则：**优先保留双方改动**。自有代码是新增的，冲突基本只会出现在挂载点，按 `[CUSTOM]` 标记手工保留即可。

## 六、脚本规范

本仓库的脚本输出文案使用中文，因此**变量展开后紧跟中文字符时，必须用 `${VAR}` 大括号形式**。

```bash
# 错误：bash 会把「（」的字节当成变量名的一部分，
# 在 set -u 下直接报 BASE_BRANCH: unbound variable
echo "更新 $BASE_BRANCH（仅快进）"

# 正确
echo "更新 ${BASE_BRANCH}（仅快进）"
```

变量后紧跟空格、英文标点或引号时，`$VAR` 简写是安全的，无需强加括号。

## 七、上游规范的处理

上游根目录的 `AGENTS.md` 约束了该项目的开发行为（例如改动前需征得用户同意）。本仓库在该文件基础上执行，**不修改它**。自有约定与上游规范冲突时，以本文件为准并在此记录原因。
