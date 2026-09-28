import assert from "node:assert/strict";
import test from "node:test";
import { isOpenRouterBaseURL, openRouterVideoBody } from "./openrouter-video";

test("OpenRouter converts the failed multipart request into native JSON", async () => {
    const body = new FormData();
    for (const [key, value] of Object.entries({model:"google/veo-3.1",prompt:"moonlit village",seconds:"4",size:"1280x720",resolution_name:"720p",preset:"normal",video_generate_audio:"false"})) body.set(key, value);
    body.append("input_reference[]", "https://media.invalid/ref.png");
    body.set("first_frame_url", "https://media.invalid/first.png");
    assert.deepEqual(await openRouterVideoBody(body), {
        model:"google/veo-3.1",prompt:"moonlit village",duration:4,size:"1280x720",resolution:"720p",generate_audio:false,
        input_references:[{type:"image_url",image_url:{url:"https://media.invalid/ref.png"}}],
        frame_images:[{type:"image_url",image_url:{url:"https://media.invalid/first.png"},frame_type:"first_frame"}],
    });
});

test("OpenRouter detection is exact and invalid duration is rejected locally", async () => {
    assert.equal(isOpenRouterBaseURL("https://openrouter.ai/api/v1"),true);
    assert.equal(isOpenRouterBaseURL("https://openrouter.ai.evil.invalid/api/v1"),false);
    assert.equal(isOpenRouterBaseURL("https://api.openai.com/v1"),false);
    const body = new FormData(); body.set("seconds","four");
    await assert.rejects(openRouterVideoBody(body), /正整数/);
});
