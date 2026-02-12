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
  content: z.string().min(1).max(10000).describe("Text content to visualize"),
  format: z.enum(["svg", "png"]).default("svg").describe("Output format"),
  style_id: z.string().optional().describe("Napkin AI style identifier"),
  language: z.string().default("en-US").describe("Language code (BCP 47)"),
  number_of_visuals: z.number().int().min(1).max(4).default(1).describe("Number of visuals to generate"),
  context_before: z.string().max(5000).optional().describe("Context before the content"),
  context_after: z.string().max(5000).optional().describe("Context after the content"),
  transparent_background: z.boolean().default(false).describe("Use transparent background"),
  inverted_color: z.boolean().default(false).describe("Use inverted/dark color mode"),
  width: z.number().int().min(100).max(4096).optional().describe("PNG width"),
  height: z.number().int().min(100).max(4096).optional().describe("PNG height"),
});

export type NapkinVisualRequest = z.infer<typeof NapkinVisualRequestSchema>;

export interface NapkinSubmitResponse {
  id?: string;
  request_id?: string;
  status: string;
  created_at: string;
}

export interface NapkinStatusResponse {
  status: "pending" | "processing" | "completed" | "failed" | "expired";
  progress?: number;
  generated_files?: NapkinFileInfo[];
  files?: NapkinFileInfo[];
  urls?: string[];
  files_ready?: number;
  files_total?: number;
  error?: string;
  message?: string;
}

export interface NapkinFileInfo {
  id: string;
  url?: string;
  format: string;
  filename?: string;
  size_bytes?: number;
  width?: number;
  height?: number;
  created_at?: string;
  checksum?: string;
}

// --- MCP Tool Input Schemas ---

export const GenerateVisualSchema = z.object({
  content: z.string().min(1).max(10000).describe("Text content to visualize"),
  format: z.enum(["svg", "png"]).default("svg").describe("Output format"),
  style_id: z.string().optional().describe("Napkin AI style identifier"),
  language: z.string().default("en-US").describe("Language code (BCP 47)"),
  number_of_visuals: z.number().int().min(1).max(4).default(1).describe("Number of visuals to generate"),
  context_before: z.string().max(5000).optional().describe("Context before the content"),
  context_after: z.string().max(5000).optional().describe("Context after the content"),
  transparent_background: z.boolean().default(false).describe("Use transparent background"),
  inverted_color: z.boolean().default(false).describe("Use inverted/dark color mode"),
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
  content: z.string().min(1).max(10000).describe("Text content to visualize"),
  format: z.enum(["svg", "png"]).default("svg").describe("Output format"),
  style_id: z.string().optional().describe("Napkin AI style identifier"),
  inverted_color: z.boolean().default(false).describe("Use inverted/dark color mode"),
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
  file_id: string;
  format: string;
  filename?: string;
  napkin_url?: string;
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
