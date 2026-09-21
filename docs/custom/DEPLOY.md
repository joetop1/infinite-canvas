# 部署说明（二次开发版）

## 一、结论

改动只发生在前端（`web/src/services/api/video.ts`），**后端代码一行未动**。
但本项目的 `Dockerfile` 把前端与后端**打进同一个镜像**，所以"重新部署"的实际动作是**重建这一个镜像**。

不需要跟着变的：

| 项目 | 说明 |
|---|---|
| 数据库 | 未新增表、未改结构，无需迁移 |
| 后端路由 | `/api/v1/videos/:id/content` 上游本来就有（`router/router.go:70`，走 `handler.AIVideoContent`），不是我们加的 |
| 环境变量 | `.env` 原样可用 |
| 数据 | `./data` 卷（SQLite、媒体文件、提示词库）原样保留 |
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

## 四、方式 B：打 tag，让 GitHub Actions 构建镜像

仓库自带上游的 `.github/workflows/docker-image.yml`（未改动），它在**推送 `v*` 标签**时自动构建 amd64 + arm64 多架构镜像，推到 `ghcr.io/joetop1/infinite-canvas`。这条路的构建跑在 GitHub 上，不占服务器资源。

```bash
git checkout custom && git pull
git tag v0.7.1-custom.1
git push origin v0.7.1-custom.1
```

注意：**标签要打在 `custom` 分支的 HEAD 上，且用新的标签名**。不要推送从上游同步下来的既有标签，那会构建上游的代码而不是你的。

构建完成后（Actions 面板可见进度），服务器侧：

```bash
# 首次需要登录 ghcr.io（包默认是私有的），用带 read:packages 权限的 PAT
echo <YOUR_PAT> | docker login ghcr.io -u joetop1 --password-stdin

docker compose -f docker-compose.custom.yml pull
docker compose -f docker-compose.custom.yml up -d
```

或者把 `docker-compose.custom.yml` 里的 `image` 换成 `ghcr.io/joetop1/infinite-canvas:<标签>`，删除 `build` 段。

## 五、部署后验证

1. 打开画布，**新建一条视频生成任务**（用 new-api 渠道）。
2. 预期：建任务 → 轮询到 `completed` → 自动取回 mp4 → 视频出现在画布上，不再报「视频生成完成但没有返回视频地址」。
3. 若仍失败，看失败文案：
   - `视频内容下载失败：404` → 你的 new-api 版本没有实现 `/v1/videos/{id}/content`，改用会直接返回 url 的旧路由 `/v1/video/generations/{id}`
   - `视频内容下载失败：401 / 403` → 渠道 API Key 或鉴权头有问题
   - 其他 → 把原文与「管理后台 → AI 日志」里的响应体一起看

验证是否真的跑上了新代码：

```bash
docker exec infinite-canvas sh -c 'grep -c "needsOpenAIContent" /app/web/.next/static/chunks/*.js 2>/dev/null | grep -v ":0"'
```

有输出（非 0 计数）说明自定义逻辑已经进了前端产物。

## 六、回滚

```bash
docker compose -f docker-compose.custom.yml down
docker compose -f docker-compose.yml up -d   # 回到上游官方镜像
```

数据在 `./data` 卷里，回滚不会丢。改回新版本同样只是重建镜像。

## 七、以后每次上游更新后

```bash
./scripts/sync-upstream.sh                       # 合并上游新版到 custom
docker compose -f docker-compose.custom.yml up -d --build
```

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

如果服务器上装的是宝塔之类的面板，**建议仍用命令行执行 compose 命令**，避免面板把 compose 文件改写或另起一套容器管理逻辑，导致实际运行的东西和仓库里的文件不一致。
