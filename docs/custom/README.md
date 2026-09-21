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

以下是唯一允许改动上游文件的位置。改动时统一加注释标记 `[CUSTOM]`，方便合并冲突时快速识别。

| 上游文件 | 允许的改动 | 备注 |
| --- | --- | --- |
| `router/router.go` | 末尾追加自有模块注册，如 `custom.Register(v1)` | 一行 |
| `main.go` | 初始化自有模块 | 2–3 行 |
| `repository/db.go` | `AutoMigrate(...)` 参数中追加自有 model | 仅在新增数据表时需要 |
| `web/src/constant/navigation-tools.ts` | 数组末尾追加菜单对象 | 导航为数据驱动，追加后桌面端与移动端同时生效 |
| `web/src/app/(user)/layout.tsx` | 仅在需要登录态初始化时改动 | 通常无需改动 |
| `web/src/services/api/video.ts` | `cacheProtectedVideo()` 中的取回分支 | 扩展「完成后另取 `/videos/{id}/content`」的协议判断，见 `CHANGELOG-custom.md` |

**除以上文件外，任何上游文件都不应出现自有改动。** 若发现必须新增挂载点，先在本文件登记，再动手。

## 四、自有代码目录约定

| 位置 | 用途 |
| --- | --- |
| `custom/` | 后端自有 Go 代码（handler / service / repository / model） |
| `web/src/app/(user)/<自有页面>/` | 前端自有页面（App Router 新增目录即新增路由） |
| `web/src/services/api/<自有模块>.ts` | 前端自有 API 封装 |
| `docs/custom/` | 本约定及相关文档 |
| `scripts/` | 自有脚本 |

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
