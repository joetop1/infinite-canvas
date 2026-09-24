# 部署说明（二次开发版）

## 零、最短路径（TL;DR）

本机数据已确认**可丢弃、可重建**，因此不需要任何数据保全措施，直接换镜像重建即可。三步：

**1) 打标签，触发云端构建**

```bash
git push
git tag v0.7.1-custom.3 && git push origin v0.7.1-custom.3
```

**2) 等 Actions 变绿**

https://github.com/joetop1/infinite-canvas/actions —— 两个 `build` 作业并行跑（amd64 / arm64），随后一个 `merge` 作业把它们合成多架构清单。实测一次完整构建约 3 分 35 秒。

**3) 宝塔面板「容器编排」里编辑配置，只改 `image` 一行**，并删掉 `build` 段与 `pull_policy: always`：

```yaml
    image: ghcr.io/joetop1/infinite-canvas:v0.7.1-custom.3
```

保存 → 重启容器。**反向代理配置不用动**（容器名与端口都未变）。

**关于镜像仓库权限**：实测确认 `ghcr.io/joetop1/infinite-canvas` **匿名可拉**（包随公开仓库自动为 Public），不需要额外授权步骤。若哪天拉取报 401/403，见第四节的补救办法。

> 为什么走 Actions 而不在服务器上构建：面板的 compose 存放在面板自己的目录，`build: context: .` 在那里没有源码和 Dockerfile，构建必然失败。原因详见第九节。

## 一、结论

改动只发生在前端（`web/src/services/api/video.ts`），**后端代码一行未动**。
但本项目的 `Dockerfile` 把前端与后端**打进同一个镜像**，所以"重新部署"的实际动作是**重建这一个镜像**。

不需要跟着变的：

| 项目 | 说明 |
|---|---|
| 数据库 | 未新增表、未改结构，无需迁移 |
| 后端路由 | `/api/v1/videos/:id/content` 上游本来就有（`router/router.go:70`，走 `handler.AIVideoContent`），不是我们加的 |
| 环境变量 | `.env` 原样可用 |
| 数据 | `./data` 卷（SQLite、媒体文件、提示词库）。本机已确认该数据**可丢弃**，因此即使挂载路径写错也不会造成损失 |
| 渠道配置 | 未新增协议，无需重选 |

前后端如何连通：浏览器请求同源的 `/api/*`，由 Next.js 的路由处理器（`web/src/app/api/[...path]/route.ts`）代理到容器内部的 Go 服务 `127.0.0.1:8080`。

## 二、部署前必须先确认：你服务器上跑的是哪个镜像

这是最容易踩的一步：

| 你用的 compose 文件 | 实际运行的镜像 | 后果 |
|---|---|---|
| `docker-compose.yml`（上游默认） | `ghcr.io/tigerowo/infinite-canvas:latest` | **不含你的任何改动**。且带 `pull_policy: always`，每次 `up` 都会拉上游最新版，把二次开发内容静默覆盖 |
| `docker-compose.custom.yml`（本仓库新增） | `infinite-canvas:custom`（源码构建） | 正确 |
| `docker-compose.local.yml`（上游提供） | `infinite-canvas:local`（源码构建） | 也能用，但没有 `container_name`，容器名会变成 `<目录名>-app-1`；从官方 compose 切过来时需先停掉旧容器，否则占着 3000 端口 |

### 一段命令查清现状

在服务器上执行（只读，不改任何东西）：

```bash
docker ps --format 'table {{.Names}}\t{{.Image}}\t{{.Ports}}'
echo "----- 画布容器是从哪个配置启起来的 -----"
docker inspect infinite-canvas --format '镜像:    {{.Config.Image}}
compose: {{index .Config.Labels "com.docker.compose.project.config_files"}}
目录:    {{index .Config.Labels "com.docker.compose.project.working_dir"}}
启动于:  {{.State.StartedAt}}' || echo "没有名为 infinite-canvas 的容器，用上面列表里的实际名字替换"
echo "----- 本机资源（决定能否在本机构建） -----"
echo "CPU 核心: $(nproc)"; free -h 2>/dev/null | head -2; df -h / | tail -1
```

第二条最关键：`compose:` 会直接告诉你正在用的是 `docker-compose.yml` 还是别的文件；`目录:` 给你仓库的实际路径。有了这两个值，后面所有命令都能确定地写出来。

## 三、方式 A：在服务器上从源码构建（最直接）

前提：服务器上有本仓库的克隆，且 `origin` 指向你自己的仓库（不是上游）。

```bash
cd /path/to/infinite-canvas

# 1) 首次：确认远程指向自己
git remote -v

# 2) 拉取改动（保留 custom 分支）
git fetch origin && git checkout custom && git pull

# 3) 确保有 .env（首次部署时需要，参考 .env.example）
ls -la .env

# 4) 从上游默认 compose 切过来时，先停掉旧容器（只做一次）
docker compose -f docker-compose.yml down

# 5) 构建并启动
docker compose -f docker-compose.custom.yml up -d --build

# 6) 看日志确认两个进程都起来了
docker compose -f docker-compose.custom.yml logs -f --tail=50
```

启动正常的标志：日志里既有 Go 服务的 gin 输出，也有 Next.js 的 `Ready` 提示。

**构建失败不会影响正在运行的旧容器**——`--build` 先构建镜像，成功后才重建容器。所以这一步是安全的。

> 本机不适用这条路：面板编排的 compose 不在仓库目录，且同机多个容器共存，`next build` 峰值 2–4 GB 内存会拖累邻居。见第九节。

## 四、方式 B：打 tag，让 GitHub Actions 构建镜像

仓库自带上游的 `.github/workflows/docker-image.yml`（未改动），它在**推送 `v*` 标签**时自动构建 amd64 + arm64 多架构镜像，推到 `ghcr.io/joetop1/infinite-canvas`。构建跑在 GitHub 的机器上，不占服务器资源。

```bash
git checkout custom && git pull
git tag v0.7.1-custom.3
git push origin v0.7.1-custom.3
```

该工作流产出两个标签，指向同一个 digest：

| 标签 | 来源 | 说明 |
|---|---|---|
| `v0.7.1-custom.3` | `type=ref,event=tag` | 与 git 标签同名，**推荐固定使用这个** |
| `<短 sha>`（如 `d13cff2`） | `type=sha,prefix=` | 按提交哈希，便于溯源 |
| `latest` | metadata-action 的 `flavor: latest=auto` 自动追加 | **每次构建都会覆盖**，是把双刃剑，见下方警告 |

### 两个必须守住的纪律

**一、标签要打在 `custom` 分支的 HEAD 上，且用新的标签名。** 不要推送从上游同步下来的既有标签（`v0.4.5` … `v0.7.1`），那会构建**上游的代码**而不是你的。

**二、不要用 `git push origin --tags` 或 `git push --tags`。** 本地存在上游的全部标签，整批推送会一次性触发多个构建，并把 `latest` 覆盖成上游版本。

> **`latest` 的取舍**：因为 `latest` 每次构建都被覆盖，把面板的 `image` 写成 `:latest` 就能实现"推送即发布"——以后再也不必改面板配置，重启容器就自动拿到新版。代价是它同时会被**上游标签**的构建覆盖。所以：
> - 想省事：用 `:latest`，但严守上面第二条纪律。
> - 想稳妥：固定写版本号（如 `:v0.7.1-custom.2`），每次发布改一行 `image`。**默认推荐这个。**

构建完成后（Actions 面板可见进度），服务器侧直接拉取即可：

```bash
docker compose -f docker-compose.custom.yml pull
docker compose -f docker-compose.custom.yml up -d
```

**关于是否需要 `docker login`**：实测 `ghcr.io/joetop1/infinite-canvas` 匿名可拉，无需登录。若哪天报 401/403（例如把包改成了私有、或换了仓库），二选一：

```bash
# 方案一：把包改回 Public（GitHub → 头像 → Your packages → infinite-canvas
#         → Package settings → Change visibility → Public）
# 方案二：用带 read:packages 权限的 PAT 登录
echo <YOUR_PAT> | docker login ghcr.io -u joetop1 --password-stdin
```

或者把 `docker-compose.custom.yml` 里的 `image` 换成 `ghcr.io/joetop1/infinite-canvas:<标签>`，删除 `build` 段。

## 五、部署后验证

### 5.1 先确认跑的是哪个镜像（决定性的一步）

```bash
docker inspect infinite-canvas --format '镜像名: {{.Config.Image}}
镜像ID: {{.Image}}
创建于: {{.Created}}'
```

**首选：查镜像的 revision 标签**（最直观——它直接说明镜像内代码对应哪个提交）

```bash
TAG=v0.7.1-custom.2      # 换成你实际部署的那个标签
git rev-parse "$TAG"      # 本地记录：这个标签指向哪个提交
docker image inspect ghcr.io/joetop1/infinite-canvas:$TAG \
  --format '{{index .Config.Labels "org.opencontainers.image.revision"}}'
```

两条命令的输出**应当完全一致**。所以不必预先背下任何哈希值，比对本身就是自洽的。

> 这里刻意不写死摘要值：每发一版哈希都会变，写死就得跟着改文档，反而容易抄错版本。
> 想知道某一版的确切摘要，见第十节「构建记录」。

**备选：比对镜像 ID**。这个方法也可用，但要知道 `docker` 显示的"镜像 ID"会随
**镜像存储后端**而变，同一个镜像可能显示成两个不同的值：

- **containerd 镜像存储**（Docker 25+ 常见）→ 显示 **多架构 index digest**
- **传统 image store** → 显示 **amd64 config digest**

两者都不是错误，只要与 registry 上对应标签的摘要一致即可。实测香港那台服务器走 containerd 存储，
`docker inspect {{.Image}}` 显示的是**多架构 index digest**（不是 amd64 config digest），
容易被误判成"ID 不对"——所以**优先用上面的 revision 标签**，可以完全绕开这个歧义。

| 看到的 | 含义 |
|---|---|
| revision 标签与 `git rev-parse <tag>` 一致 | 已经是自建镜像，容器跑的就是该提交的代码 |
| 镜像名仍是 `ghcr.io/tigerowo/infinite-canvas:latest` | 面板配置没改成，或改完没重启 |
| 镜像名对，但 revision 标签为空或对不上 | 标签或架构不对（误用 `:latest`、或拉到 arm64 那份） |

> **不要在容器里 grep 自定义标识符来验证代码是否生效。**
>
> 这条弯路已经走过一次：`docker exec ... grep needsOpenAIContent /app/web/.next/static/chunks/*.js`
> **无论镜像新旧都必然无输出**。原因是前端产物经过 minify，局部变量名会被重命名成短名，
> 注释也会被剥离——源码里的标识符在里面根本不存在。
>
> 判断"跑的是哪个版本"只有三把可靠的尺子：**镜像的 revision 标签**、**镜像 ID**、以及**实际功能**。

### 5.2 功能验证

1. 打开画布，**新建一条视频生成任务**（用 new-api 渠道）。
2. 预期：建任务 → 轮询到 `completed` → 自动取回 mp4 → 视频出现在画布上，不再报「视频生成完成但没有返回视频地址」。
3. 若仍失败，**失败文案本身就是诊断信息**（`v0.7.1-custom.2` 起会带上完整 URL 与状态码）：

| 文案 | 含义 | 下一步 |
|---|---|---|
| `HTTP 404 @ .../videos/{id}/content` | new-api 没实现这条新路由 | 代码会自动回退旧路由，若这行仍出现说明**两个路由都不存在**，去看 new-api 版本 |
| `新路由 HTTP 404 …；旧路由 HTTP 404 …` | 新旧路由都取不到该任务 | 任务可能真的过期/被上游清理，**换一条新任务再试** |
| `HTTP 401 / 403` | 渠道 API Key 或鉴权头不对 | 检查画布里的渠道配置 |
| 其他 | — | 把原文与「管理后台 → AI 日志」里的响应体一起看 |

> **注意"过期"与"路由不存在"的区别**：过期任务拿旧 ID 去测，新旧路由都会报 404，
> 分不清是路由没实现还是文件被清。要判断路由是否存在，**用一条刚建的新任务**，
> 或用一个格式合法但不存在的假 ID——后者的 404 一定是"路由或任务不存在"，与过期无关。

### 5.3 Fal.ai / Replicate 渠道验证（`v0.7.1-custom.3` 起）

这两个平台**不需要重建镜像就能验证通不通**，因为它不走画布后端代理：
请求由浏览器直连平台，画布后端只提供参数转译。所以先在界面上把渠道建好再测一次生成。

1. 「设置 → 模型渠道 → 新增」：协议选 **Fal.ai**，API Key 粘贴 fal 后台的原始 key，保存。
2. 在模型列表里**手动输入** `fal-ai/flux/dev`（该渠道没有模型列表接口，会提示手填，属正常）。
3. 画布跑一次**纯文生图**，只填 prompt，不要加参考图——这是最小请求。
4. 预期：几秒到几十秒后出图。

| 现象 | 原因 | 下一步 |
|---|---|---|
| 出图 | 通了 | 再试带参考图的模型、视频模型、Replicate |
| `401` / `Invalid API key` | key 不对 | Fal 直接粘贴原始 key（程序会补 `Key ` 前缀）；Replicate 用 `r8_` 开头的 token |
| `422` / 字段非法 | 该模型输入 schema 与默认映射不同 | 用模型名 query 指定，如 `fal-ai/flux/dev?image_size=landscape_16_9` |
| `Fal 任务缺少请求 ID 或模型 ID` | 模型名没填或只填了参数 | 见 `FAL-REPLICATE.md` 第三节 |
| 浏览器控制台报 CORS | 浏览器直连被拦 | 正常网络下不应出现，两者官方 SDK 都支持浏览器直连 |
| `Fal 渠道暂不支持参考音频` | 附了音频参考 | 改用参考图片或参考视频 |

详细的模型名格式、字段映射表、字段名覆盖键见 **`FAL-REPLICATE.md`**。

> **图片尺寸不会自动传**：画布的宽高比选择器对 Fal/Replicate 的**图片**接口不生效
> （Fal 用 `image_size` 枚举、Replicate 各模型不同，猜错字段名会让请求 422）。
> 视频接口的尺寸会自动折成 `aspect_ratio`。需要指定图片尺寸请用 query，例如
> `fal-ai/flux/dev?image_size=landscape_16_9`。

## 六、回滚

```bash
docker compose -f docker-compose.custom.yml down
docker compose -f docker-compose.yml up -d   # 回到上游官方镜像
```

数据在 `./data` 卷里，回滚不会丢。改回新版本同样只是重建镜像。

## 七、以后每次上游更新后

```bash
./scripts/sync-upstream.sh                       # 合并上游新版到 custom
git push                                         # 推代码
git tag v0.7.1-custom.3 && git push origin v0.7.1-custom.3   # 触发构建（标签递增）
```

等 Actions 变绿，再按第零节第 3 步重启容器。

## 八、多项目共存的主机上要注意什么

本机不止跑这一个容器时，下面几件事需要留意。

### 端口

画布容器内部固定监听 3000，宿主机侧端口可由 `CANVAS_HOST_PORT` 覆盖：

```bash
# 若 3000 已被别的服务占用，在项目目录的 .env 里加一行
echo 'CANVAS_HOST_PORT=3100' >> .env
```

先看端口是否已被占用：

```bash
docker ps --filter "publish=3000" --format '{{.Names}}\t{{.Ports}}'
ss -lntp 2>/dev/null | grep ':3000 ' || true
```

**只要容器名和端口都不变，反向代理（Nginx / Caddy / 宝塔）的配置就完全不用动。** 上游的 compose 与 `docker-compose.custom.yml` 用的都是 `container_name: infinite-canvas`，正是为了让这次替换对代理层透明。反过来，如果改用上游提供的 `docker-compose.local.yml`，容器名会变成 `<目录名>-app-1`，代理若按容器名解析就会断掉——这也是新增这个 compose 文件的原因。

### 资源

构建是本项目最吃资源的时刻，跑起来的运行占用反而不高。经验值：

| 资源 | 建议下限 | 说明 |
|---|---|---|
| 内存 | 4 GB | `bun install` + `next build` 峰值 2–4 GB，低于 3 GB 容易在 next build 阶段被 OOM Kill |
| 磁盘 | 15 GB 可用 | 三层构建缓存 + 镜像，构建完可 `docker builder prune` 回收 |
| CPU | 2 核 | 单核也能构建，只是慢 |

**如果服务器内存不足 4 GB，优先用第四节的「方式 B」让 GitHub Actions 构建镜像**，服务器只负责 `pull`，不承担构建开销。

查看当前占用：

```bash
docker stats --no-stream --format 'table {{.Name}}\t{{.MemUsage}}\t{{.CPUPerc}}'
```

### 清理空间时的坑

不要用 `docker system prune -a`——它会连自建的 `infinite-canvas:custom` 镜像一起删掉，下次 `up` 就得重新构建。只清构建缓存：

```bash
docker builder prune -f --filter until=168h
```

### 香港节点的便利

拉 Docker Hub、`ghcr.io`、以及从 GitHub `git clone` / `git pull` 通常都可直连，不需要配镜像加速或代理。这也是这台机器适合用「方式 A 在服务器上构建」的原因之一。

### 用面板管理容器时

如果服务器上装的是宝塔面板、并且用它的「容器编排」功能管理这个容器，见第九节——那种环境下方式 A（服务器构建）实际上不可用，需要走方式 B。

## 九、宝塔面板「容器编排」场景

面板管理的容器与命令行管理的有三个实质差别，需要针对性处理。

### 1. compose 文件在面板目录里，不在你的仓库里

面板会把它保存的 compose 内容放在自己的目录下运行，因此：

- `build: context: .` 里的 `.` 指的是**面板目录**，那里没有源码和 Dockerfile。这个 `build` 段一旦真被执行就会失败。
- 结论：**面板环境下不要在服务器上构建**，直接走第四节的「方式 B」——由 GitHub Actions 构建镜像，面板只负责拉取。

### 2. 相对路径的数据卷会跟着 compose 文件位置走

```yaml
volumes:
  - ./data:/app/data
```

`./data` 是相对于 **compose 文件所在目录**解析的，不是相对于你的仓库。所以换一个目录跑 compose，`./data` 就指向另一个空目录，应用会以全新的空数据库启动。

**本机当前无数据（2026-09-21 确认），这个风险暂时不构成实际损失。** 但只要开始正常使用，画布项目、历史素材、渠道配置就会落在 `./data` 里，届时规则就变成硬的：

- 从命令行接管时，第一步是**先查清当前实际的挂载源路径**，再把新 compose 的 `volumes` 指向同一个绝对路径：

```bash
docker inspect infinite-canvas --format '{{range .Mounts}}{{.Type}}  {{.Source}}  ->  {{.Destination}}{{"\n"}}{{end}}'
```

- 改配置时**不要动 `volumes` 和 `env_file`**，只动 `image`，路径不变即零数据风险。

### 3. 端口以容器实际映射为准

面板里显示的 compose 文本可能与容器实际运行状态不一致（改过配置但没重新创建容器时尤其如此）。**以 `docker port` 的输出为准**：

```bash
docker port infinite-canvas
```

反向代理指向的是宿主机侧那个端口。改配置时端口必须与反代目标一致，否则改完域名就访问不通。

### 推荐的操作序列（方式 B + 面板）

即第零节的三步，这里展开细节。

1. 打标签，让 GitHub Actions 构建镜像：

```bash
git tag v0.7.1-custom.3 && git push origin v0.7.1-custom.3
```

2. 等 Actions 跑完（仓库 Actions 面板可见进度，两个架构各一次构建，最后合成多架构清单）。
3. 在面板的「容器编排」里编辑配置，把 `image` 换成自建镜像，**删掉 `build` 段与 `pull_policy: always`**，其余保持原样：

```yaml
services:
  app:
    image: ghcr.io/joetop1/infinite-canvas:v0.7.1-custom.3
    container_name: infinite-canvas
    env_file:
      - .env
    volumes:
      - ./data:/app/data
    ports:
      - "8091:3000"
    restart: unless-stopped
```

   面板里的**原始**内容（供逐行对照）：

```yaml
services:
  app:
    image: ghcr.io/tigerowo/infinite-canvas:latest
    build:
      context: .
      dockerfile: Dockerfile
    pull_policy: always
    container_name: infinite-canvas
    env_file:
      - .env
    volumes:
      - ./data:/app/data
    ports:
      - "8091:3000"
    restart: unless-stopped
```

   只有三处变化：`image:` 换值、**删掉 `build:` 整段（两行）**、**删掉 `pull_policy:`**。其余全部保持原样，**特别是 `ports` 必须是 `"8091:3000"`**——反向代理 `canvas.moirapis.com` 指向的是宿主机的 8091，改了这个端口域名立刻访问不通。

4. 保存后点「更新镜像」或「重启」。此后面板的「更新镜像」按钮会去拉你自己的镜像，成为发布新版本的正常入口。

### 为什么必须删掉 `pull_policy: always`

上游默认配置里那一行的作用是：每次启动都把 `image:` 指定的镜像重新拉一遍。而 `image:` 指向的是上游官方镜像 `ghcr.io/tigerowo/infinite-canvas:latest`。两者叠加的结果是——**只要面板重启过容器，二次开发的改动就会被上游最新版覆盖回去**，而且不会有任何提示。这是"我明明改了但没生效"的最常见原因。

（删掉它之后仍有正常的拉取行为：当 `image:` 指定的标签本地不存在时，`docker compose up` 会自动去拉。所以换成版本号标签后不需要这一行。）

## 十、构建记录

本节是「某个版本的确切摘要是什么」的权威来源。第 5.1 节之所以不写死哈希，就是因为它应该来这里查。

想查**任意标签**的实际摘要（只读，不需要任何凭据）：

```bash
TAG=v0.7.1-custom.3
TOKEN=$(curl -s "https://ghcr.io/token?scope=repository:joetop1/infinite-canvas:pull&service=ghcr.io" \
  | python3 -c "import json,sys; print(json.load(sys.stdin)['token'])")
curl -sI -H "Authorization: Bearer $TOKEN" \
  -H "Accept: application/vnd.oci.image.index.v1+json" \
  "https://ghcr.io/v2/joetop1/infinite-canvas/manifests/$TAG" \
  | grep -i docker-content-digest
```

### v0.7.1-custom.3 — 2026-09-24

| 项目 | 值 |
|---|---|
| 触发 | 推送标签 `v0.7.1-custom.3`（`push` 事件） |
| 运行 | [Actions run 35951053343](https://github.com/joetop1/infinite-canvas/actions/runs/35951053343) |
| 源码 | `d13cff2d1dbe77263efe57d6e7b31da692b5e732` |
| 结果 | 全部成功，4 个作业：`meta` → `build (amd64)` / `build (arm64)` → `merge` |
| 耗时 | 约 4 分 16 秒 |
| 镜像 | `ghcr.io/joetop1/infinite-canvas:v0.7.1-custom.3` |
| 多架构 digest | `sha256:5e7a537d9c41e617995ffd03a926b2ce7234ce3adb9b2e14d0c6e9aa5579456a` |
| 可见性 | 匿名可拉（无需登录） |

内容：新增 Fal.ai / Replicate 两条直连协议（图片与视频），不改动既有协议的报文与地址。
使用说明见 `FAL-REPLICATE.md`。

**本次构建出现过一次 amd64 偶发失败，值得记下来**：首次推送标签（run `35950649958`）时
`build (linux/amd64)` 在 `RUN bun run build` 步骤以 **exit code 132（SIGILL，非法指令）**
失败，而同一份源码的 arm64 作业成功。删除并重新推送同一标签后**两个架构均成功**，
因此判定为 runner 侧的偶发问题（`Illegal instruction` 通常是 Next.js 的 SWC 原生二进制
在不支持相应指令集的机器上崩溃），与源码无关。

> 处理这类失败的顺序：**先看是哪一步失败、退出码是多少**（`/actions/runs/<id>/jobs` 看步骤，
> `/check-runs/<id>/annotations` 能拿到失败行与错误原文，无需登录凭据）；
> **只有两个架构同时失败**或**重跑后仍在同一步失败**，才去怀疑自己的改动。

各层摘要（`docker` 显示的"镜像 ID"会随**镜像存储后端**而异，见 5.1 节说明）：

| 层级 | 摘要 |
|---|---|
| 多架构 index（总清单） | `sha256:5e7a537d9c41e617995ffd03a926b2ce7234ce3adb9b2e14d0c6e9aa5579456a` |
| linux/amd64 子 manifest | `sha256:2842fad32db0a2ba29b9a1242bfc70d7ef122156de7cd28d139f63a99d81ab29` |
| linux/amd64 config digest | `sha256:70e9552dc42aa3f2318f5eed201e81716cb17eb2579953668f072363d12d37a0` |
| linux/arm64 子 manifest | `sha256:d0336edc7cb204aba9e058e0e7472b4bea71f5af71068f712931eb5ef377fcfd` |

镜像内元数据（已核验）：

```
org.opencontainers.image.revision = d13cff2d1dbe77263efe57d6e7b31da692b5e732
org.opencontainers.image.version  = v0.7.1-custom.3
org.opencontainers.image.created  = 2026-09-24T03:20:45.973Z
```

> 注意：打标签之后又推了一次**纯文档**提交（`c276bab`，更新挂载点清单与说明），
> 它**不在镜像里**，也不影响功能。`custom` 分支 HEAD 因此比标签新一个文档提交，
> 这是有意的——镜像对应的是功能提交。

`latest` 标签本次也指向 custom.3。但**不要依赖它**，理由见第四节的纪律。

### v0.7.1-custom.2 — 2026-09-21

| 项目 | 值 |
|---|---|
| 触发 | 推送标签 `v0.7.1-custom.2`（`push` 事件） |
| 运行 | [Actions run 35614493207](https://github.com/joetop1/infinite-canvas/actions/runs/35614493207) |
| 源码 | `0b34718fb0f12ccb5bc9af6412b2e06a84293930` |
| 结果 | 全部成功，4 个作业：`meta` → `build (amd64)` / `build (arm64)` → `merge` |
| 耗时 | 约 4 分 30 秒（14:47:11 → 14:51:41 UTC）；amd64 作业用时较长（约 4 分 2 秒） |
| 镜像 | `ghcr.io/joetop1/infinite-canvas:v0.7.1-custom.2` |
| 多架构 digest | `sha256:e919dc8e76b9e28305a034413c117ee63adb16a3e6f8ba1518d119682cb0a7b7` |
| 可见性 | 匿名可拉（无需登录） |

相对 `v0.7.1-custom.1` 的差异：新增 `fetchVideoContent()` 路由回退与可诊断错误文案。

三个标签实测指向**同一个 digest**，可任选：

| 标签 | 来源 | 说明 |
|---|---|---|
| `v0.7.1-custom.2` | `type=ref,event=tag` | **推荐固定使用这个** |
| `0b34718` | `type=sha,prefix=` | 按提交哈希，便于溯源 |
| `latest` | metadata-action 自动追加 | 每次构建都被覆盖，见第四节的纪律 |

各层摘要。注意 `docker` 显示的"镜像 ID"会随**镜像存储后端**而异，见 5.1 节说明：

| 层级 | 摘要 |
|---|---|
| 多架构 index（总清单） | `sha256:e919dc8e76b9e28305a034413c117ee63adb16a3e6f8ba1518d119682cb0a7b7` |
| linux/amd64 子 manifest | `sha256:eac9525674785339b51bb5f39e2d2cf6030d638f20d487ba16aa892f3e6ab1a8` |
| linux/amd64 config digest | `sha256:9af285171193fe155a137f689f6825cab47f14d99841dafa8ffdc02579f76aae` |
| linux/arm64 子 manifest | `sha256:8253753215c4c4d978626c558febc21ee3d7eb800f83778a3177bd4938168523` |

镜像内元数据（已核验，证明构建来源正确）：

```
org.opencontainers.image.revision = 0b34718fb0f12ccb5bc9af6412b2e06a84293930
org.opencontainers.image.source   = https://github.com/joetop1/infinite-canvas
org.opencontainers.image.version  = v0.7.1-custom.2
```

### v0.7.1-custom.1 — 2026-09-21

| 项目 | 值 |
|---|---|
| 触发 | 推送标签 `v0.7.1-custom.1`（`push` 事件） |
| 运行 | [Actions run 35610027095](https://github.com/joetop1/infinite-canvas/actions/runs/35610027095) |
| 源码 | `308647a548d9d69111f9764867064ab7ca524c23` |
| 结果 | 全部成功，4 个作业：`meta` → `build (amd64)` / `build (arm64)` → `merge` |
| 耗时 | 约 3 分 35 秒（14:07:31 → 14:11:06 UTC） |
| 镜像 | `ghcr.io/joetop1/infinite-canvas:v0.7.1-custom.1` |
| 多架构 digest | `sha256:ce57220a73910dcd72ba3ffbc404eedb2092ca9affd2afc7e548b4be1f4d47f8` |
| 可见性 | 匿名可拉（HTTP 200），无需登录 |

各层摘要。注意 `docker` 显示的"镜像 ID"会随**镜像存储后端**而异，下面两个值都可能出现：

| 层级 | 摘要 |
|---|---|
| 多架构 index（总清单） | `sha256:ce57220a73910dcd72ba3ffbc404eedb2092ca9affd2afc7e548b4be1f4d47f8` |
| linux/amd64 子 manifest | `sha256:f0e1a4e47ad9d6bb99772452890fe374023f87cf35e12c77d896f8ad2ed6d15b` |
| linux/amd64 config digest | `sha256:963539197b64d8ece976eba986bf30f8d33e027916c022a208e6c6bb3a6224e3` |
| linux/arm64 子 manifest | `sha256:83cc64920d3bf7cb0f4c2af4a086a0704f99aea61518cf9e3137c1cbb13dfc87` |

- **containerd 镜像存储**（Docker 25+ 常见）→ 显示 **index digest**
- **传统 image store** → 显示 **config digest**

香港那台服务器是 x86_64、走 containerd 存储，实测 `docker inspect {{.Image}}` 显示 index digest `ce57220a…`，与上表首行吻合。

镜像内元数据（已核验，证明构建来源正确）：

```
org.opencontainers.image.revision = 308647a548d9d69111f9764867064ab7ca524c23
org.opencontainers.image.source   = https://github.com/joetop1/infinite-canvas
org.opencontainers.image.version  = v0.7.1-custom.1
```

验证包可见性的办法（不需要凭据）：

```bash
TOKEN=$(curl -s "https://ghcr.io/token?scope=repository:joetop1/infinite-canvas:pull&service=ghcr.io" \
  | python3 -c "import json,sys; print(json.load(sys.stdin)['token'])")
curl -s -o /dev/null -w "%{http_code}\n" -H "Authorization: Bearer $TOKEN" \
  -H "Accept: application/vnd.oci.image.index.v1+json" \
  "https://ghcr.io/v2/joetop1/infinite-canvas/manifests/latest"
```

返回 `200` = 公开可拉；`401`/`403` = 私有，需按第四节处理。
