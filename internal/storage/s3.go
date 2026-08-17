package storage

import (
	"time"

	"context"
	"fmt"
	"io"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"

	"github.com/chaosapp/backend/internal/config"
)

// S3 stores files in AWS S3 or any S3-compatible service (R2, MinIO) via
// a custom endpoint. Credentials come from the standard AWS env/chain.
type S3 struct {
	client  *s3.Client
	bucket  string
	baseURL string
}

var _ Storage = (*S3)(nil)

func NewS3(ctx context.Context, cfg config.StorageConfig) (*S3, error) {
	awsCfg, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(cfg.S3Region))
	if err != nil {
		return nil, fmt.Errorf("load aws config: %w", err)
	}
	client := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		if cfg.S3Endpoint != "" {
			o.BaseEndpoint = aws.String(cfg.S3Endpoint) // R2 / MinIO
			o.UsePathStyle = true
		}
	})
	base := cfg.PublicBaseURL
	if base == "" {
		base = fmt.Sprintf("https://%s.s3.%s.amazonaws.com", cfg.S3Bucket, cfg.S3Region)
	}
	return &S3{client: client, bucket: cfg.S3Bucket, baseURL: strings.TrimRight(base, "/")}, nil
}

func (s *S3) Put(ctx context.Context, key string, r io.Reader, contentType string) (string, error) {
	_, err := s.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(s.bucket),
		Key:         aws.String(key),
		Body:        r,
		ContentType: aws.String(contentType),
	})
	if err != nil {
		return "", fmt.Errorf("s3 put: %w", err)
	}
	return s.baseURL + "/" + key, nil
}

func (s *S3) Delete(ctx context.Context, key string) error {
	_, err := s.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	return err
}

// List pages through the bucket under prefix. Object storage is billed per
// request, so the sweeper that calls this runs on a slow timer.
func (s *S3) List(ctx context.Context, prefix string) ([]Object, error) {
	var out []Object
	paginator := s3.NewListObjectsV2Paginator(s.client, &s3.ListObjectsV2Input{
		Bucket: aws.String(s.bucket),
		Prefix: aws.String(prefix),
	})
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("list objects: %w", err)
		}
		for _, obj := range page.Contents {
			if obj.Key == nil {
				continue
			}
			var modified time.Time
			if obj.LastModified != nil {
				modified = *obj.LastModified
			}
			out = append(out, Object{Key: *obj.Key, Modified: modified})
		}
	}
	return out, nil
}
