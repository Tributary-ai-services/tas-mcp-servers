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
  accessKey: process.env.MINIO_ACCESS_KEY || "minioadmin",
  secretKey: process.env.MINIO_SECRET_KEY || "minioadmin123",
  bucket: process.env.MINIO_BUCKET || "napkin-visuals",
  region: process.env.MINIO_REGION || "us-east-1",
};

export class MinioClient {
  private client: S3Client;
  private defaultBucket: string;

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

    this.defaultBucket = config.bucket;
  }

  async ensureBucket(bucket?: string): Promise<void> {
    const bucketName = bucket || this.defaultBucket;
    try {
      await this.client.send(new HeadBucketCommand({ Bucket: bucketName }));
    } catch (error: any) {
      if (error.name === "NotFound" || error.$metadata?.httpStatusCode === 404) {
        await this.client.send(new CreateBucketCommand({ Bucket: bucketName }));
        console.error(`Created bucket: ${bucketName}`);
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
    const bucketName = bucket || this.defaultBucket;
    await this.ensureBucket(bucketName);

    const result = await this.client.send(
      new PutObjectCommand({
        Bucket: bucketName,
        Key: key,
        Body: data,
        ContentType: contentType,
      })
    );

    const url = `${config.endpoint}/${bucketName}/${key}`;

    return {
      bucket: bucketName,
      key,
      url,
      size_bytes: data.length,
      etag: result.ETag,
    };
  }

  async download(key: string, bucket?: string): Promise<Buffer> {
    const bucketName = bucket || this.defaultBucket;
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
    const bucketName = bucket || this.defaultBucket;
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
    const bucketName = bucket || this.defaultBucket;
    await this.client.send(
      new DeleteObjectCommand({
        Bucket: bucketName,
        Key: key,
      })
    );
  }

  getDefaultBucket(): string {
    return this.defaultBucket;
  }
}
