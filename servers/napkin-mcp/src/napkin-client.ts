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
    const body: Record<string, any> = {
      content: request.content,
      format: request.format || "svg",
      language: request.language || "en-US",
      number_of_visuals: request.number_of_visuals || 1,
      transparent_background: request.transparent_background || false,
      inverted_color: request.inverted_color || false,
    };
    if (request.style_id) body.style_id = request.style_id;
    if (request.context_before) body.context_before = request.context_before;
    if (request.context_after) body.context_after = request.context_after;
    if (request.width) body.width = request.width;
    if (request.height) body.height = request.height;

    const response = await this.client.post("/v1/visual", body);
    return response.data;
  }

  getRequestId(resp: NapkinSubmitResponse): string {
    return resp.id || resp.request_id || "";
  }

  async getVisualStatus(requestId: string): Promise<NapkinStatusResponse> {
    const response = await this.client.get(`/v1/visual/${requestId}/status`);
    const data = response.data;

    // Normalize file arrays: prefer generated_files > files > urls
    if (!data.generated_files && data.files) {
      data.generated_files = data.files;
    } else if (!data.generated_files && !data.files && data.urls) {
      data.generated_files = data.urls.map((u: string) => ({ url: u, id: "", format: "" }));
    }

    return data;
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

      if (status.status === "expired") {
        throw new Error(`Napkin visual request expired for request ${requestId}`);
      }

      await this.sleep(backoffMs);
      backoffMs = Math.min(backoffMs * 1.5, maxBackoff);
    }

    throw new Error(
      `Napkin visual generation timed out after ${this.maxWaitTime}ms for request ${requestId}`
    );
  }

  async downloadFileById(requestId: string, fileId: string): Promise<Buffer> {
    const response = await this.client.get(`/v1/visual/${requestId}/file/${fileId}`, {
      responseType: "arraybuffer",
      timeout: 60000,
    });
    return Buffer.from(response.data);
  }

  async downloadFile(url: string): Promise<Buffer> {
    const response = await this.client.get(url, {
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
