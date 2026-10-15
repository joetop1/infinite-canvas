---
title: 接口响应约定
description: 业务接口统一响应结构与前端处理约定
---

# 接口响应约定

## Fal MiniMax H3 Max 全能参考

- `minimax/h3-max/reference-to-video` 的普通参考素材使用 `reference_image_urls`、`reference_video_urls`、`reference_audio_urls` 数组，单份输入也保持数组；`image_url` 和 `end_image_url` 只用于明确指定的首尾帧，不能把普通参考图片当作首帧。
- 至少一份普通参考，最多 9 图／3 视频／3 音频，总计 12 份；`duration` 为 0.92–15 秒的数值，清晰度为 `480P`／`768P`／`1080P`，旧的 720p 选择映射到原生 768P，默认 `prompt_expansion_mode=balanced`。原生音频不提供独立开关，界面不显示无效的声音开关。
- 参数错误在调用模型前返回；具体参考素材参数按 [Fal 官方 Schema](https://fal.ai/models/minimax/h3-max/reference-to-video/api) 适配，不套用通用单数素材字段。

## Seedance 参考素材交付

- Seedance 原生 `content` 和 MainAPIs/New API `metadata.content` 的本站图片、视频和音频引用，在持久化任务的幂等提交回调内转换为其 S3/R2 存储对象的预签名 GET 链接，有效期 24 小时；保留 `role`、顺序及其他生成参数，不更改外部直链。
- 使用已配置的对象所属存储签名，不复制素材到其他服务商，不公开整个存储桶。模型提交前以 Range GET 检查实际签名链接；无法签名／读取时返回明确素材准备错误，按提交拒绝规则返还预扣，不静默回退到本站下载地址。
- AI 调用日志记录实际提交链接；链接含限时读取权限，只能按现有管理员日志权限查看，勿公开分享。

## Fal 视频编辑素材与结果

- 本站素材域名按浏览器 Origin／Referer、原请求 Host 和 X-Forwarded-Host 转发链识别；已匹配本站文件记录但域名无法识别时，在模型提交前明确拒绝，禁止静默绕过转存。
- 官方 Fal 账号渠道的 Kling O3 Edit 在幂等提交回调内，将本站 `/api/files/:id/content` 原片及参考图转存到 Fal 官方素材存储，请求 24 小时生命周期后使用返回的文件地址。素材准备失败作为明确拒绝处理，模型任务尚未提交，站内预扣按提交失败规则返还。
- Fal 队列的 `COMPLETED` 表示处理结束，仍须读取结果接口。结果返回确定的 4xx（除 408/429）时标记 `failed`，不得一直返回 `processing`；网络、限流及服务端临时故障保留 `retrieving` 并有限退避，达到上限后要求核对上游任务。

后端业务接口统一返回 JSON：

```json
{
  "code": 0,
  "data": {},
  "msg": "ok"
}
```

- `code`: 业务状态码，`0` 表示成功，非 `0` 表示失败。
- `data`: 业务数据。失败时通常为 `null`。
- `msg`: 响应消息。成功默认为 `ok`，失败时放错误原因。

前端请求逻辑以 `code` 判断业务是否成功。当前后端业务失败也会返回 HTTP 200，前端不要只依赖 HTTP 状态码判断结果。

接口连接失败、服务不可达、返回体不是约定 JSON 时，前端按网络或接口异常处理。

## AutoDL 渠道

- `POST /api/ai/autodl/workflows` 使用上述业务响应结构。请求为 `{ baseUrl, workflowId? }`，不传上游 API Key；不带工作流 ID 返回列表，带 ID 返回详情。元数据包含 `uuid`、`name`、`kind` 和详情的 `input_rules`，不返回上游内部节点映射。
- AutoDL 视频继续使用现有 `/api/v1/videos` 创建、查询及任务响应结构。
- `/api/v1/audio/speech` 是 AI 协议代理入口。AutoDL 成功时返回 `{ provider: "autodl", audio_url, mime_type }`；失败仍使用现有 `Fail` 业务结构。其他语音协议的响应保持原样。
- 现有 `/api/v1/canvas/audio-tasks` 消费 AutoDL 的上游音频 URL，返回既有任务结构，不新增结果下载或转存。

## 渠道自定传参转译

- 沿用 `/api/v1/images/generations`、`/api/v1/images/edits`、`/api/v1/audio/speech`、`/api/v1/videos` 及现有画布任务入口。命中自定义的账号请求携带 `{ model, n, _parameterTranslation: { kind, variables } }`；视频另携带面板 `seconds`，沿用现有按秒费用计算。`kind` 为 `image`、`video`、`audio`。
- `_parameterTranslation` 是系统内部负载。后端按账号或公共渠道读取保存的原文，转译后只向供应商发送配置里的方法、地址、请求头、查询参数和请求体。
- 图片 JSON 结果归一为 `{ data: [{ url }] }`；音频 URL 归一为 `{ provider: "parameter-translation", audio_url }`。配置直接返回文件时沿用二进制响应及 MIME。失败仍用现有 `Fail` 业务结构。
- 视频沿用现有任务响应，增加 `parameter_translation: true` 标记；私有查询快照和公共渠道脚本不随响应返回。创建、查询、完成和失败均进入现有任务管理，失败不重发默认请求。

## 视频裁剪另存

- `POST /api/v1/media/trim-video` 需要用户登录，multipart 字段为 `file`、`start`、`end`（秒）。选择片段为 3–14.9 秒，原片不超过 200MB / 3840px。成功返回 `video/mp4` 二进制，前端沿用素材上传入口另存；失败仍返回现有 `Fail` 业务结构。
- 裁剪不调用 AI 渠道、不创建生成任务或扣算力点；FFmpeg 最多并发两个任务，90 秒超时，并再次读取输出编码的真实长度。原视频保留，不自动提交编辑生成。


## 统一模型规范

- 管理员 `POST /api/admin/settings/channel-contracts` 接收 `{ index, channel }`，每次最多 30 个所选模型，沿用 `{ code, data, msg }`；客户端每批 20 个。服务端使用保存的渠道凭据读取官方规范，不返回密钥。响应沿用渠道模型目录结构，元数据 `generation_contract` 包含状态、来源、修订摘要和语义字段约束，不公开编译器或完整私有请求。
- 渠道 `strictContracts` 默认 false。开启后，后端不信任客户端能力声明，按服务端规范映射并完整校验生成请求；前端禁用待适配模型。Fal/Replicate 至少有一个已就绪模型才能开启；自定义参数转译不可同时启用。原生 Seedance 使用核对后的本地规范。
- 参考素材匿名下载预检仅检查 GET/Range 可读性，不扩大素材分享或新增转存。失败返回中文原因，不提交模型；视频沿用已有幂等任务和预扣返还逻辑。Replicate 固定到规范对应版本，避免同步后 latest_version 变化。
