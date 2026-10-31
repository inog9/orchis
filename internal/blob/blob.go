// Package blob menyimpan objek (keluaran mentah, bukti) di penyimpanan kompatibel S3 (MinIO).
package blob

import (
	"bytes"
	"context"
	"fmt"
	"io"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

type Store struct {
	c      *minio.Client
	bucket string
}

type Options struct {
	Endpoint, AccessKey, SecretKey, Bucket string
	TLS                                    bool
}

func Open(ctx context.Context, o Options) (*Store, error) {
	c, err := minio.New(o.Endpoint, &minio.Options{Creds: credentials.NewStaticV4(o.AccessKey, o.SecretKey, ""), Secure: o.TLS})
	if err != nil {
		return nil, err
	}
	ok, err := c.BucketExists(ctx, o.Bucket)
	if err != nil {
		return nil, err
	}
	if !ok {
		if err := c.MakeBucket(ctx, o.Bucket, minio.MakeBucketOptions{}); err != nil {
			return nil, err
		}
	}
	return &Store{c: c, bucket: o.Bucket}, nil
}

func (s *Store) Put(ctx context.Context, key string, data []byte, contentType string) error {
	_, err := s.c.PutObject(ctx, s.bucket, key, bytes.NewReader(data), int64(len(data)), minio.PutObjectOptions{ContentType: contentType})
	return err
}

// Get membaca objek (dipakai tes dan unduhan keluaran mentah).
func (s *Store) Get(ctx context.Context, key string) ([]byte, error) {
	o, err := s.c.GetObject(ctx, s.bucket, key, minio.GetObjectOptions{})
	if err != nil {
		return nil, err
	}
	defer o.Close()
	data, err := io.ReadAll(io.LimitReader(o, (32<<20)+1))
	if err != nil {
		return nil, err
	}
	if len(data) > 32<<20 {
		return nil, fmt.Errorf("object exceeds download limit")
	}
	return data, nil
}

// Ping memeriksa bahwa penyimpanan objek dan bucket-nya terjangkau.
func (s *Store) Ping(ctx context.Context) error {
	ok, err := s.c.BucketExists(ctx, s.bucket)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("bucket %q tidak ada", s.bucket)
	}
	return nil
}
