import { useUserStore } from "@/stores/use-user-store";

export async function trimVideoFile(file: Blob, start: number, end: number, signal: AbortSignal) {
    if (file.size > 200 * 1024 * 1024) throw new Error("原视频不能超过 200MB");
    const token = useUserStore.getState().token;
    if (!token) throw new Error("请先登录后使用视频裁剪");
    const body = new FormData();
    body.append("file", file, "source-video");
    body.append("start", String(start));
    body.append("end", String(end));
    const response = await fetch("/api/v1/media/trim-video", { method: "POST", headers: { Authorization: `Bearer ${token}` }, body, signal });
    if (!response.ok || !response.headers.get("content-type")?.startsWith("video/")) {
        const payload = await response.json().catch(() => null);
        throw new Error(payload?.msg || `视频裁剪失败：${response.status}`);
    }
    return response.blob();
}
