package s3

import (
	"context"
	"io"
	"net/url"
	"strings"

	"github.com/arencloud/hermes/internal/models"

	minio "github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

type Client struct{ mc *minio.Client }

// Stat returns object info (size, content type) if available.
func (c *Client) Stat(ctx context.Context, bucket, key string) (minio.ObjectInfo, error) {
	return c.mc.StatObject(ctx, bucket, key, minio.StatObjectOptions{})
}

// DownloadWithInfo returns a reader for the object and best-effort total size.
// It attempts StatObject first; if that fails, it will still return the reader
// and try to Stat via the object's Stat() once the stream is established.
func (c *Client) DownloadWithInfo(ctx context.Context, bucket, key string) (io.ReadCloser, int64, error) {
	// Try Stat first for size
	if info, err := c.mc.StatObject(ctx, bucket, key, minio.StatObjectOptions{}); err == nil {
		rc, err2 := c.mc.GetObject(ctx, bucket, key, minio.GetObjectOptions{})
		if err2 != nil {
			return nil, 0, err2
		}
		return rc, info.Size, nil
	}
	// Fallback: open object and attempt Stat() on it
	obj, err := c.mc.GetObject(ctx, bucket, key, minio.GetObjectOptions{})
	if err != nil {
		return nil, 0, err
	}
	if oi, err := obj.Stat(); err == nil {
		return obj, oi.Size, nil
	}
	return obj, 0, nil
}

func normalizeEndpoint(endpoint string, useSSL bool) (host string, secure bool) {
	secure = useSSL
	if endpoint == "" {
		return "", secure
	}
	// If endpoint contains scheme, parse and strip it; prefer scheme over useSSL flag
	if strings.HasPrefix(endpoint, "http://") || strings.HasPrefix(endpoint, "https://") {
		if u, err := url.Parse(endpoint); err == nil {
			if u.Scheme == "https" {
				secure = true
			} else if u.Scheme == "http" {
				secure = false
			}
			// Keep host:port as endpoint for minio.New
			return u.Host, secure
		}
	}
	return endpoint, secure
}

func forcePathStyle(p models.Provider) bool {
	// Use path-style for non-AWS by default; AWS prefers virtual-hosted
	pt := strings.ToLower(strings.TrimSpace(p.Type))
	return pt == "minio" || pt == "mcg" || pt == "generic" || pt == "" // default to path style for unknown
}

func NewFromProvider(p models.Provider) (*Client, error) {
	endpoint, secure := normalizeEndpoint(p.Endpoint, p.UseSSL)
	opts := &minio.Options{
		Creds:  credentials.NewStaticV4(p.AccessKey, p.SecretKey, ""),
		Secure: secure,
		Region: p.Region,
	}
	// minio-go v7 automatically handles path-style for custom endpoints (MinIO/MCG).
	// For AWS, virtual-hosted style is used by default.
	mc, err := minio.New(endpoint, opts)
	if err != nil {
		return nil, err
	}
	return &Client{mc: mc}, nil
}

func (c *Client) ListBuckets(ctx context.Context) ([]minio.BucketInfo, error) {
	return c.mc.ListBuckets(ctx)
}

func (c *Client) CreateBucket(ctx context.Context, name string, region string) error {
	return c.mc.MakeBucket(ctx, name, minio.MakeBucketOptions{Region: region})
}

func (c *Client) DeleteBucket(ctx context.Context, name string) error {
	return c.mc.RemoveBucket(ctx, name)
}

func (c *Client) ListObjects(ctx context.Context, bucket, prefix string, recursive bool) ([]minio.ObjectInfo, error) {
	var out []minio.ObjectInfo
	for obj := range c.mc.ListObjects(ctx, bucket, minio.ListObjectsOptions{Prefix: prefix, Recursive: recursive}) {
		if obj.Err != nil {
			return nil, obj.Err
		}
		out = append(out, obj)
	}
	return out, nil
}

func (c *Client) Upload(ctx context.Context, bucket, key string, reader io.Reader, size int64, contentType string) (minio.UploadInfo, error) {
	opts := minio.PutObjectOptions{ContentType: contentType}
	return c.mc.PutObject(ctx, bucket, key, reader, size, opts)
}

func (c *Client) Download(ctx context.Context, bucket, key string) (io.ReadCloser, error) {
	return c.mc.GetObject(ctx, bucket, key, minio.GetObjectOptions{})
}

func (c *Client) DeleteObject(ctx context.Context, bucket, key string) error {
	return c.mc.RemoveObject(ctx, bucket, key, minio.RemoveObjectOptions{})
}

func (c *Client) CopyObject(ctx context.Context, srcBucket, srcKey, dstBucket, dstKey string) error {
	src := minio.CopySrcOptions{Bucket: srcBucket, Object: srcKey}
	dst := minio.CopyDestOptions{Bucket: dstBucket, Object: dstKey}
	_, err := c.mc.CopyObject(ctx, dst, src)
	return err
}

func (c *Client) MoveObject(ctx context.Context, srcBucket, srcKey, dstBucket, dstKey string) error {
	if err := c.CopyObject(ctx, srcBucket, srcKey, dstBucket, dstKey); err != nil {
		return err
	}
	return c.DeleteObject(ctx, srcBucket, srcKey)
}
