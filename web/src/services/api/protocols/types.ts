export type DirectVideoResponse = { id: string; task_id?: string; video_id?: string; status?: string; progress?: number; video_url?: string; url?: string; error?: { message?: string }; model?: string };

export type DirectProtocolAdapter = Readonly<{
    rawAuthorization?: boolean;
    // 需要自定义鉴权前缀时使用（如 Fal 的 `Key <token>`）。未提供时按 rawAuthorization 决定是否加 `Bearer `。
    authorization?(apiKey: string): string;
    pollURL?(baseUrl: string, taskId: string, model?: string): string;
    pollPath(taskId: string): string;
    readTaskId(payload: unknown): string;
    readCreatedImageURLs?(payload: unknown): string[];
    readCreatedVideoStatus(payload: unknown): string;
    readImagePoll(payload: unknown): { urls: string[]; done: boolean; error: string };
    readVideoPoll(payload: unknown, pollId: string, model: string): DirectVideoResponse;
    readAudioPoll?(payload: unknown): { url: string; done: boolean; error: string };
    readError(payload: unknown): string;
}>;
