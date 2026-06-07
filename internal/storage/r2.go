package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

var ErrStorageNotConfigured = errors.New("R2 storage is not configured")

// MediaStore defines object storage operations for user media.
type MediaStore interface {
	PresignGetURL(key string, expiry time.Duration) (string, error)
	PresignPutURL(key string, expiry time.Duration, contentType string) (string, error)
	PutObject(ctx context.Context, key string, body io.Reader, size int64, contentType string) error
	DeleteObject(ctx context.Context, key string) error
	PublicURL(key string) string
	PublicDomain() string
}

// R2Config holds Cloudflare R2 connection settings.
type R2Config struct {
	AccountID     string
	AccessKeyID   string
	SecretKey     string
	BucketName    string
	PublicDomain  string
}

// R2Store implements MediaStore against Cloudflare R2.
type R2Store struct {
	client       *s3.Client
	presigner    *s3.PresignClient
	bucket       string
	publicDomain string
}

// NewR2Store creates an R2-backed MediaStore.
func NewR2Store(cfg R2Config) (*R2Store, error) {
	if cfg.AccountID == "" || cfg.AccessKeyID == "" || cfg.SecretKey == "" {
		return nil, fmt.Errorf("R2 account ID and credentials are required")
	}
	if cfg.BucketName == "" {
		cfg.BucketName = "geoloc-media"
	}

	endpoint := fmt.Sprintf("https://%s.r2.cloudflarestorage.com", cfg.AccountID)
	client := s3.New(s3.Options{
		Region: "auto",
		Credentials: credentials.NewStaticCredentialsProvider(
			cfg.AccessKeyID,
			cfg.SecretKey,
			"",
		),
		BaseEndpoint: aws.String(endpoint),
		UsePathStyle: true,
	})

	return &R2Store{
		client:       client,
		presigner:    s3.NewPresignClient(client),
		bucket:       cfg.BucketName,
		publicDomain: strings.TrimSuffix(cfg.PublicDomain, "/"),
	}, nil
}

// NewR2StoreFromEnv builds an R2Store from environment variables.
func NewR2StoreFromEnv() (*R2Store, error) {
	cfg := R2Config{
		AccountID:    os.Getenv("R2_ACCOUNT_ID"),
		AccessKeyID:  os.Getenv("R2_ACCESS_KEY_ID"),
		SecretKey:    os.Getenv("R2_SECRET_ACCESS_KEY"),
		BucketName:   os.Getenv("R2_BUCKET_NAME"),
		PublicDomain: os.Getenv("R2_PUBLIC_DOMAIN"),
	}
	if cfg.BucketName == "" {
		cfg.BucketName = "geoloc-media"
	}
	return NewR2Store(cfg)
}

func (s *R2Store) PresignGetURL(key string, expiry time.Duration) (string, error) {
	if err := ValidateKey(key); err != nil {
		return "", err
	}
	result, err := s.presigner.PresignGetObject(context.Background(), &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	}, s3.WithPresignExpires(expiry))
	if err != nil {
		return "", fmt.Errorf("presign get: %w", err)
	}
	return result.URL, nil
}

func (s *R2Store) PresignPutURL(key string, expiry time.Duration, contentType string) (string, error) {
	if err := ValidateKey(key); err != nil {
		return "", err
	}
	if !AllowedImageContentTypes[contentType] {
		return "", fmt.Errorf("invalid content type: %s", contentType)
	}
	result, err := s.presigner.PresignPutObject(context.Background(), &s3.PutObjectInput{
		Bucket:      aws.String(s.bucket),
		Key:         aws.String(key),
		ContentType: aws.String(contentType),
	}, s3.WithPresignExpires(expiry))
	if err != nil {
		return "", fmt.Errorf("presign put: %w", err)
	}
	return result.URL, nil
}

func (s *R2Store) PutObject(ctx context.Context, key string, body io.Reader, size int64, contentType string) error {
	if err := ValidateKey(key); err != nil {
		return err
	}
	_, err := s.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:        aws.String(s.bucket),
		Key:           aws.String(key),
		Body:          body,
		ContentLength: aws.Int64(size),
		ContentType:   aws.String(contentType),
		CacheControl:  aws.String("public, max-age=31536000, immutable"),
	})
	if err != nil {
		return fmt.Errorf("put object: %w", err)
	}
	return nil
}

func (s *R2Store) DeleteObject(ctx context.Context, key string) error {
	if err := ValidateKey(key); err != nil {
		return err
	}
	_, err := s.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return fmt.Errorf("delete object: %w", err)
	}
	return nil
}

func (s *R2Store) PublicURL(key string) string {
	if key == "" || s.publicDomain == "" {
		return ""
	}
	return s.publicDomain + "/" + key
}

func (s *R2Store) PublicDomain() string {
	return s.publicDomain
}

// NoopMediaStore returns errors for all operations when R2 is not configured.
type NoopMediaStore struct{}

func NewNoopMediaStore() *NoopMediaStore {
	return &NoopMediaStore{}
}

func (n *NoopMediaStore) PresignGetURL(string, time.Duration) (string, error) {
	return "", ErrStorageNotConfigured
}

func (n *NoopMediaStore) PresignPutURL(string, time.Duration, string) (string, error) {
	return "", ErrStorageNotConfigured
}

func (n *NoopMediaStore) PutObject(context.Context, string, io.Reader, int64, string) error {
	return ErrStorageNotConfigured
}

func (n *NoopMediaStore) DeleteObject(context.Context, string) error {
	return ErrStorageNotConfigured
}

func (n *NoopMediaStore) PublicURL(key string) string {
	return ""
}

func (n *NoopMediaStore) PublicDomain() string {
	return ""
}

// MemoryMediaStore is an in-memory MediaStore for tests.
type MemoryMediaStore struct {
	objects      map[string][]byte
	publicDomain string
}

func NewMemoryMediaStore(publicDomain string) *MemoryMediaStore {
	return &MemoryMediaStore{
		objects:      make(map[string][]byte),
		publicDomain: strings.TrimSuffix(publicDomain, "/"),
	}
}

func (m *MemoryMediaStore) PresignGetURL(key string, _ time.Duration) (string, error) {
	if err := ValidateKey(key); err != nil {
		return "", err
	}
	if _, ok := m.objects[key]; !ok {
		return "", fmt.Errorf("object not found")
	}
	if m.publicDomain != "" {
		return m.publicDomain + "/" + key + "?signed=1", nil
	}
	return "memory:///" + key, nil
}

func (m *MemoryMediaStore) PresignPutURL(key string, _ time.Duration, contentType string) (string, error) {
	if err := ValidateKey(key); err != nil {
		return "", err
	}
	if !AllowedImageContentTypes[contentType] {
		return "", fmt.Errorf("invalid content type: %s", contentType)
	}
	return "memory://put/" + key, nil
}

func (m *MemoryMediaStore) PutObject(_ context.Context, key string, body io.Reader, _ int64, _ string) error {
	if err := ValidateKey(key); err != nil {
		return err
	}
	data, err := io.ReadAll(body)
	if err != nil {
		return err
	}
	m.objects[key] = data
	return nil
}

func (m *MemoryMediaStore) DeleteObject(_ context.Context, key string) error {
	if err := ValidateKey(key); err != nil {
		return err
	}
	delete(m.objects, key)
	return nil
}

func (m *MemoryMediaStore) PublicURL(key string) string {
	if key == "" || m.publicDomain == "" {
		return ""
	}
	return m.publicDomain + "/" + key
}

func (m *MemoryMediaStore) PublicDomain() string {
	return m.publicDomain
}
