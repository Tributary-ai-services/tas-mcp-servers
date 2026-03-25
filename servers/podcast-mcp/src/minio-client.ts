import {
  S3Client,
  PutObjectCommand,
  GetObjectCommand,
  ListObjectsV2Command,
  DeleteObjectCommand,
  CreateBucketCommand,
  HeadBucketCommand,
} from "@aws-sdk/client-s3";
import { MinioUploadResult, MinioListResult } from "./types";

const config = {
  endpoint: process.env.MINIO_ENDPOINT || "http://tas-minio-shared:9000",
  publicUrl: process.env.MINIO_PUBLIC_URL || "",
  accessKey: process.env.MINIO_ACCESS_KEY || "minioadmin",
  secretKey: process.env.MINIO_SECRET_KEY || "minioadmin123",
  assetsBucket: process.env.MINIO_ASSETS_BUCKET || "podcast-assets",
  outputBucket: process.env.MINIO_OUTPUT_BUCKET || "podcast-output",
  region: process.env.MINIO_REGION || "us-east-1",
};

export class MinioClient {
  private client: S3Client;

  constructor() {
    this.client = new S3Client({
      endpoint: config.endpoint,
      region: config.region,
      credentials: {
        accessKeyId: config.accessKey,
        secretAccessKey: config.secretKey,
      },
      forcePathStyle: true,
    });
  }

  async ensureBucket(bucket: string): Promise<void> {
    try {
      await this.client.send(new HeadBucketCommand({ Bucket: bucket }));
    } catch (error: any) {
      if (error.name === "NotFound" || error.$metadata?.httpStatusCode === 404) {
        await this.client.send(new CreateBucketCommand({ Bucket: bucket }));
        console.error(`Created bucket: ${bucket}`);
      } else {
        throw error;
      }
    }
  }

  async upload(
    key: string,
    data: Buffer,
    contentType: string,
    bucket?: string
  ): Promise<MinioUploadResult> {
    const bucketName = bucket || config.outputBucket;
    await this.ensureBucket(bucketName);

    const result = await this.client.send(
      new PutObjectCommand({
        Bucket: bucketName,
        Key: key,
        Body: data,
        ContentType: contentType,
      })
    );

    const baseUrl = config.publicUrl || config.endpoint;
    const url = `${baseUrl}/${bucketName}/${key}`;

    return {
      bucket: bucketName,
      key,
      url,
      size_bytes: data.length,
      etag: result.ETag,
    };
  }

  async download(key: string, bucket?: string): Promise<Buffer> {
    const bucketName = bucket || config.outputBucket;
    const result = await this.client.send(
      new GetObjectCommand({
        Bucket: bucketName,
        Key: key,
      })
    );

    const chunks: Uint8Array[] = [];
    const stream = result.Body as any;
    for await (const chunk of stream) {
      chunks.push(chunk);
    }
    return Buffer.concat(chunks);
  }

  async list(prefix?: string, bucket?: string, limit?: number): Promise<MinioListResult[]> {
    const bucketName = bucket || config.outputBucket;
    const result = await this.client.send(
      new ListObjectsV2Command({
        Bucket: bucketName,
        Prefix: prefix,
        MaxKeys: limit || 20,
      })
    );

    return (result.Contents || []).map((obj) => ({
      key: obj.Key || "",
      size: obj.Size || 0,
      last_modified: obj.LastModified?.toISOString() || "",
      etag: obj.ETag,
    }));
  }

  async delete(key: string, bucket?: string): Promise<void> {
    const bucketName = bucket || config.outputBucket;
    await this.client.send(
      new DeleteObjectCommand({
        Bucket: bucketName,
        Key: key,
      })
    );
  }

  getAssetsBucket(): string {
    return config.assetsBucket;
  }

  getOutputBucket(): string {
    return config.outputBucket;
  }
}
