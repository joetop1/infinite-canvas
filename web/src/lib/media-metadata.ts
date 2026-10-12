export type MediaMetadata = { durationMs: number; width?: number; height?: number };
const metadataCache = new Map<string, Promise<MediaMetadata>>();

// Read the media stream's duration, never the time spent generating it.
export function readMediaMetadata(url: string, kind: "video" | "audio"): Promise<MediaMetadata> {
    const key = `${kind}:${url}`;
    const cached = metadataCache.get(key);
    if (cached) return cached;
    const pending = new Promise<MediaMetadata>((resolve, reject) => {
        const media = document.createElement(kind);
        const cleanup = () => {
            clearTimeout(timer);
            media.onloadedmetadata = null;
            media.onerror = null;
            media.removeAttribute("src");
            media.load();
        };
        const fail = () => { cleanup(); reject(new Error("无法读取参考素材的实际时长，请确认素材可以播放后重试")); };
        const timer = setTimeout(fail, 15000);
        media.preload = "metadata";
        media.onloadedmetadata = () => {
            if (!Number.isFinite(media.duration) || media.duration <= 0) { fail(); return; }
            const metadata: MediaMetadata = { durationMs: Math.round(media.duration * 1000) };
            if (media instanceof HTMLVideoElement) { metadata.width = media.videoWidth; metadata.height = media.videoHeight; }
            cleanup();
            resolve(metadata);
        };
        media.onerror = fail;
        media.src = url;
    });
    metadataCache.set(key, pending);
    void pending.catch(() => metadataCache.delete(key));
    return pending;
}
