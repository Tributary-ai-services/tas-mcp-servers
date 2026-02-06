import { z } from "zod";

// --- Napkin AI API Types ---

export const NapkinStyleSchema = z.object({
  id: z.string().describe("Unique style identifier"),
  name: z.string().describe("Human-readable style name"),
  description: z.string().optional().describe("Style description"),
  category: z.string().optional().describe("Style category"),
  preview_url: z.string().optional().describe("Preview image URL"),
});

export type NapkinStyle = z.infer<typeof NapkinStyleSchema>;

export const NapkinVisualRequestSchema = z.object({
  content: z.string().min(1).max(50000).describe("Text content to visualize"),
  format: z.enum(["svg", "png", "ppt"]).default("svg").describe("Output format"),
  style_id: z.string().optional().describe("Napkin AI style identifier"),
  color_mode: z.enum(["light", "dark", "both"]).default("light").describe("Color mode"),
  language: z.string().default("en").describe("Language code (BCP 47)"),
  variations: z.number().int().min(1).max(5).default(1).describe("Number of variations"),
  context: z.string().optional().describe("Additional context for generation"),
});

export type NapkinVisualRequest = z.infer<typeof NapkinVisualRequestSchema>;

export interface NapkinSubmitResponse {
  id: string;
  status: string;
  created_at: string;
}

export interface NapkinStatusResponse {
  id: string;
  status: "pending" | "processing" | "completed" | "failed";
  progress?: number;
  files?: NapkinFileInfo[];
  error?: string;
  created_at: string;
  completed_at?: string;
}

export interface NapkinFileInfo {
  index: number;
  format: string;
  color_mode: string;
  url: string;
  size_bytes?: number;
  expires_at?: string;
}

// --- MCP Tool Input Schemas ---

export const GenerateVisualSchema = z.object({
  content: z.string().min(1).max(50000).describe("Text content to visualize"),
  format: z.enum(["svg", "png", "ppt"]).default("svg").describe("Output format (svg, png, or ppt)"),
  style_id: z.string().optional().describe("Napkin AI style identifier"),
  color_mode: z.enum(["light", "dark", "both"]).default("light").describe("Color mode"),
  language: z.string().default("en").describe("Language code (BCP 47)"),
  variations: z.number().int().min(1).max(5).default(1).describe("Number of variations to generate"),
  context: z.string().optional().describe("Additional context for generation"),
});

export const CheckVisualStatusSchema = z.object({
  request_id: z.string().describe("Napkin AI request ID to check status of"),
});

export const DownloadVisualSchema = z.object({
  minio_key: z.string().describe("MinIO object key for the visual"),
  bucket: z.string().default("napkin-visuals").describe("MinIO bucket name"),
});

export const ListVisualsSchema = z.object({
  prefix: z.string().optional().describe("Object key prefix to filter by"),
  bucket: z.string().default("napkin-visuals").describe("MinIO bucket name"),
  limit: z.number().int().min(1).max(100).default(20).describe("Maximum number of results"),
});

export const CreateNapkinVisualCRSchema = z.object({
  name: z.string().describe("Name for the NapkinVisual custom resource"),
  namespace: z.string().default("tas-mcp-servers").describe("Kubernetes namespace"),
  content: z.string().min(1).max(50000).describe("Text content to visualize"),
  format: z.enum(["svg", "png", "ppt"]).default("svg").describe("Output format"),
  style_id: z.string().optional().describe("Napkin AI style identifier"),
  color_mode: z.enum(["light", "dark", "both"]).default("light").describe("Color mode"),
});

// --- MinIO Types ---

export interface MinioUploadResult {
  bucket: string;
  key: string;
  url: string;
  size_bytes: number;
  etag?: string;
}

export interface MinioListResult {
  key: string;
  size: number;
  last_modified: string;
  etag?: string;
}

// --- Generated Visual Result ---

export interface GeneratedVisualFile {
  index: number;
  format: string;
  color_mode: string;
  napkin_url: string;
  minio_key: string;
  minio_url: string;
  size_bytes: number;
}

export interface GenerateVisualResult {
  request_id: string;
  status: string;
  files: GeneratedVisualFile[];
  total_time_ms: number;
}
