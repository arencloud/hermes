package s3svc

import (
	"context"
	"io"
	"net/http"

	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"

	"github.com/arencloud/hermes/internal/models"
)

type Client struct {
	s3 *s3.Client
}

func FromConfig(cfg *models.S3Storage) *Client {
	creds := credentials.NewStaticCredentialsProvider(cfg.AccessKey, cfg.SecretKey, "")
	lo := s3.Options{
		Credentials: creds,
		Region:      valueOr(cfg.Region, "us-east-1"),
	}
	if cfg.Endpoint != "" {
		lo.BaseEndpoint = &cfg.Endpoint
		lo.UsePathStyle = true
	}
	if !cfg.UseSSL {
		// If endpoint is http, SDK picks it up from BaseEndpoint
	}
	client := s3.New(lo)
	return &Client{s3: client}
}

func valueOr(v, d string) string { if v == "" { return d } ; return v }

func (c *Client) ListObjects(ctx context.Context, bucket, prefix string) ([]string, error) {
	var keys []string
	p := s3.NewListObjectsV2Paginator(c.s3, &s3.ListObjectsV2Input{Bucket: &bucket, Prefix: &prefix})
	for p.HasMorePages() {
		out, err := p.NextPage(ctx)
		if err != nil { return nil, err }
		for _, obj := range out.Contents {
			if obj.Key != nil { keys = append(keys, *obj.Key) }
		}
	}
	return keys, nil
}

func (c *Client) Upload(ctx context.Context, bucket, key string, r io.Reader, size int64, contentType string) error {
	_, err := c.s3.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      &bucket,
		Key:         &key,
		Body:        r,
		ContentType: &contentType,
	})
	return err
}

func (c *Client) Download(ctx context.Context, bucket, key string) (io.ReadCloser, int64, string, error) {
	out, err := c.s3.GetObject(ctx, &s3.GetObjectInput{Bucket: &bucket, Key: &key})
	if err != nil { return nil, 0, "", err }
	length := int64(0)
	if out.ContentLength != nil { length = *out.ContentLength }
	ct := ""
	if out.ContentType != nil { ct = *out.ContentType } else { ct = http.DetectContentType([]byte{}) }
	return out.Body, length, ct, nil
}

func (c *Client) Delete(ctx context.Context, bucket, key string) error {
	_, err := c.s3.DeleteObject(ctx, &s3.DeleteObjectInput{Bucket: &bucket, Key: &key})
	return err
}
