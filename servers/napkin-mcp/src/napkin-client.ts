import axios, { AxiosInstance } from "axios";
import {
  NapkinVisualRequest,
  NapkinSubmitResponse,
  NapkinStatusResponse,
  NapkinStyle,
} from "./types";

const config = {
  apiKey: process.env.NAPKIN_API_KEY || "",
  baseUrl: process.env.NAPKIN_API_BASE_URL || "https://api.napkin.ai",
  pollingInterval: parseInt(process.env.NAPKIN_POLLING_INTERVAL || "3000", 10),
  maxWaitTime: parseInt(process.env.NAPKIN_MAX_WAIT_TIME || "300000", 10),
};

export class NapkinClient {
  private client: AxiosInstance;
  private pollingInterval: number;
  private maxWaitTime: number;

  constructor() {
    if (!config.apiKey) {
      console.error("Warning: NAPKIN_API_KEY is not set");
    }

    this.client = axios.create({
      baseURL: config.baseUrl,
      headers: {
        "Content-Type": "application/json",
        Authorization: `Bearer ${config.apiKey}`,
      },
      timeout: 30000,
    });

    this.pollingInterval = config.pollingInterval;
    this.maxWaitTime = config.maxWaitTime;
  }

  async submitVisual(request: NapkinVisualRequest): Promise<NapkinSubmitResponse> {
    const response = await this.client.post("/v1/visual", {
      content: request.content,
      format: request.format,
      style_id: request.style_id,
      color_mode: request.color_mode,
      language: request.language,
      variations: request.variations,
      context: request.context,
    });
    return response.data;
  }

  async getVisualStatus(requestId: string): Promise<NapkinStatusResponse> {
    const response = await this.client.get(`/v1/visual/${requestId}/status`);
    return response.data;
  }

  async waitForCompletion(requestId: string): Promise<NapkinStatusResponse> {
    const startTime = Date.now();
    let backoffMs = this.pollingInterval;
    const maxBackoff = 30000;

    while (Date.now() - startTime < this.maxWaitTime) {
      const status = await this.getVisualStatus(requestId);

      if (status.status === "completed") {
        return status;
      }

      if (status.status === "failed") {
        throw new Error(`Napkin visual generation failed: ${status.error || "Unknown error"}`);
      }

      await this.sleep(backoffMs);
      backoffMs = Math.min(backoffMs * 1.5, maxBackoff);
    }

    throw new Error(
      `Napkin visual generation timed out after ${this.maxWaitTime}ms for request ${requestId}`
    );
  }

  async downloadFile(url: string): Promise<Buffer> {
    const response = await axios.get(url, {
      responseType: "arraybuffer",
      timeout: 60000,
    });
    return Buffer.from(response.data);
  }

  async listStyles(): Promise<NapkinStyle[]> {
    const response = await this.client.get("/v1/styles");
    return response.data.styles || response.data;
  }

  private sleep(ms: number): Promise<void> {
    return new Promise((resolve) => setTimeout(resolve, ms));
  }
}
