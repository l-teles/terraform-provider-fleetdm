package fleetdm

import (
	"context"
	"fmt"
	"io"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// S3Source represents an S3 object location.
type S3Source struct {
	Bucket      string
	Key         string
	Region      string // optional
	EndpointURL string // optional, for S3-compatible services (LocalStack, MinIO)
}

// DownloadS3Object downloads an object from S3 and returns its content.
func DownloadS3Object(ctx context.Context, src S3Source) ([]byte, error) {
	var opts []func(*config.LoadOptions) error
	if src.Region != "" {
		opts = append(opts, config.WithRegion(src.Region))
	}

	cfg, err := config.LoadDefaultConfig(ctx, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}

	var s3Opts []func(*s3.Options)
	if src.EndpointURL != "" {
		s3Opts = append(s3Opts, func(o *s3.Options) {
			o.BaseEndpoint = aws.String(src.EndpointURL)
			o.UsePathStyle = true // needed for LocalStack, MinIO, and test servers
		})
	}
	client := s3.NewFromConfig(cfg, s3Opts...)

	output, err := client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(src.Bucket),
		Key:    aws.String(src.Key),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to download s3://%s/%s: %w", src.Bucket, src.Key, err)
	}
	defer func() { _ = output.Body.Close() }()

	data, err := io.ReadAll(output.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read S3 object body: %w", err)
	}

	return data, nil
}
