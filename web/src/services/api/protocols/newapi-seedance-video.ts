// New API 的豆包视频适配器读取 metadata.content，不读取 input_reference[]。
export function newAPISeedanceVideoBody(body: Record<string, unknown>) {
    const images = (body["input_reference[]"] || []) as string[];
    const videos = (body["video_reference[]"] || []) as string[];
    const audios = (body["audio_reference[]"] || []) as string[];
    const firstFrame = body.first_frame_url as string | undefined;
    const lastFrame = body.last_frame_url as string | undefined;
    if (lastFrame && !firstFrame) throw new Error("请先指定首帧图片");
    if ((firstFrame || lastFrame) && (images.length || videos.length || audios.length)) {
        throw new Error("Seedance 首尾帧不能与普通参考图片、视频或音频混用，请选择一种生成方式");
    }
    const media = (kind: string, url: string, role: string) => ({ type: `${kind}_url`, [`${kind}_url`]: { url }, role });
    const content = [
        ...images.map((url) => media("image", url, "reference_image")),
        ...(firstFrame ? [media("image", firstFrame, "first_frame")] : []),
        ...(lastFrame ? [media("image", lastFrame, "last_frame")] : []),
        ...videos.map((url) => media("video", url, "reference_video")),
        ...audios.map((url) => media("audio", url, "reference_audio")),
    ];
    return {
        model: body.model,
        prompt: body.prompt,
        ...(body.seconds === -1 ? {} : { seconds: body.seconds }),
        metadata: {
            content,
            ratio: firstFrame ? "adaptive" : body.size,
            resolution: body.resolution_name,
            generate_audio: body.video_generate_audio,
            watermark: body.video_watermark,
            ...(body.seconds === -1 ? { duration: -1 } : {}),
        },
    };
}
