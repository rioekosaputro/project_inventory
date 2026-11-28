package storage

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// S3Config mengatur koneksi ke penyimpanan S3-compatible (Supabase Storage, MinIO, AWS S3).
type S3Config struct {
	Endpoint       string // mis. https://<ref>.supabase.co/storage/v1/s3 atau http://localhost:9000
	Region         string // default "us-east-1" jika kosong
	Bucket         string // mis. "erp-files"
	AccessKey      string
	SecretKey      string
	ForcePathStyle bool // true untuk Supabase Storage & MinIO
}

// S3Storage mengimplementasikan Uploader dengan AWS SDK v2 S3.
type S3Storage struct {
	client    *s3.Client
	presigner *s3.PresignClient
	bucket    string
}

// NewS3 membuat S3Storage yang siap dipakai untuk Supabase Storage, MinIO, maupun AWS S3.
func NewS3(cfg S3Config) (*S3Storage, error) {
	bucket := strings.TrimSpace(cfg.Bucket)
	if bucket == "" {
		return nil, errors.New("storage s3: S3_BUCKET wajib diisi")
	}
	if strings.TrimSpace(cfg.AccessKey) == "" || strings.TrimSpace(cfg.SecretKey) == "" {
		return nil, errors.New("storage s3: S3_ACCESS_KEY dan S3_SECRET_KEY wajib diisi")
	}
	region := strings.TrimSpace(cfg.Region)
	if region == "" {
		region = "us-east-1"
	}

	opts := s3.Options{
		Region:       region,
		Credentials:  credentials.NewStaticCredentialsProvider(cfg.AccessKey, cfg.SecretKey, ""),
		UsePathStyle: cfg.ForcePathStyle,
	}
	if ep := strings.TrimSpace(cfg.Endpoint); ep != "" {
		opts.BaseEndpoint = aws.String(ep)
	}

	client := s3.New(opts)
	return &S3Storage{
		client:    client,
		presigner: s3.NewPresignClient(client),
		bucket:    bucket,
	}, nil
}

var _ Uploader = (*S3Storage)(nil)

func (s *S3Storage) Upload(ctx context.Context, folder string, r io.Reader) (*UploadResult, error) {
	prep, err := prepareUpload(folder, r)
	if err != nil {
		return nil, err
	}

	_, err = s.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:        aws.String(s.bucket),
		Key:           aws.String(prep.key),
		Body:          bytes.NewReader(prep.data),
		ContentType:   aws.String(prep.contentType),
		ContentLength: aws.Int64(int64(len(prep.data))),
	})
	if err != nil {
		return nil, fmt.Errorf("storage s3: put object: %w", err)
	}

	return &UploadResult{
		Key:         prep.key,
		ContentType: prep.contentType,
		Size:        int64(len(prep.data)),
	}, nil
}

func (s *S3Storage) Delete(ctx context.Context, key string) error {
	cleanKey, err := sanitizeKey(key)
	if err != nil {
		return err
	}
	_, err = s.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(cleanKey),
	})
	if err != nil {
		return fmt.Errorf("storage s3: delete object: %w", err)
	}
	return nil
}

func (s *S3Storage) SignedURL(ctx context.Context, key string, ttl time.Duration) (string, error) {
	cleanKey, err := sanitizeKey(key)
	if err != nil {
		return "", err
	}
	req, err := s.presigner.PresignGetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(cleanKey),
	}, s3.WithPresignExpires(normalizeTTL(ttl)))
	if err != nil {
		return "", fmt.Errorf("storage s3: presign get: %w", err)
	}
	return req.URL, nil
}

func (s *S3Storage) SignedUploadURL(ctx context.Context, key string, contentType string, ttl time.Duration) (string, error) {
	cleanKey, err := sanitizeKey(key)
	if err != nil {
		return "", err
	}
	ct := strings.ToLower(strings.TrimSpace(contentType))
	if _, ok := allowedMIMETypes[ct]; !ok {
		return "", fmt.Errorf("%w: %s", ErrInvalidMimeType, contentType)
	}
	req, err := s.presigner.PresignPutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(s.bucket),
		Key:         aws.String(cleanKey),
		ContentType: aws.String(ct),
	}, s3.WithPresignExpires(normalizeTTL(ttl)))
	if err != nil {
		return "", fmt.Errorf("storage s3: presign put: %w", err)
	}
	return req.URL, nil
}
