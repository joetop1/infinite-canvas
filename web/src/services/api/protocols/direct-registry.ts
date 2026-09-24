import type { DirectAIProvider } from "@/lib/model-channel";
import { apimartDirectProtocol } from "./apimart";
import { arkDirectProtocol } from "./ark";
import { autodlDirectProtocol } from "./autodl";
import { falDirectProtocol } from "./fal";
import { kieDirectProtocol } from "./kie";
import { replicateDirectProtocol } from "./replicate";
import type { DirectProtocolAdapter } from "./types";

export const directProtocolAdapters: Readonly<Record<DirectAIProvider, DirectProtocolAdapter>> = {
    kie: kieDirectProtocol,
    apimart: apimartDirectProtocol,
    autodl: autodlDirectProtocol,
    ark: arkDirectProtocol,
    fal: falDirectProtocol,
    replicate: replicateDirectProtocol,
};
