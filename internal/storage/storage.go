// Package storage issues presigned PUT URLs for mission data: S3 in AWS, a signed local folder otherwise.
package storage

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// Storage presigns uploads.
type Storage interface {
	PresignPut(ctx context.Context, key string) (url string, err error)
}

// S3 presigns PUTs against a bucket.
type S3 struct {
	Bucket string
	pre    *s3.PresignClient
}

// NewS3 builds an S3 presigner from the default AWS config.
func NewS3(ctx context.Context, bucket string) (*S3, error) {
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		return nil, err
	}
	return &S3{Bucket: bucket, pre: s3.NewPresignClient(s3.NewFromConfig(cfg))}, nil
}

// PresignPut implements Storage.
func (s *S3) PresignPut(ctx context.Context, key string) (string, error) {
	r, err := s.pre.PresignPutObject(ctx, &s3.PutObjectInput{Bucket: &s.Bucket, Key: &key}, s3.WithPresignExpires(10*time.Minute))
	if err != nil {
		return "", err
	}
	return r.URL, nil
}

// Local stores uploads in a folder and signs URLs with an HMAC (presigned-URL equivalent).
type Local struct {
	Dir, Secret, PublicBase string
}

func (l *Local) sign(key string) string {
	m := hmac.New(sha256.New, []byte(l.Secret))
	m.Write([]byte(key))
	return hex.EncodeToString(m.Sum(nil))
}

// PresignPut implements Storage.
func (l *Local) PresignPut(_ context.Context, key string) (string, error) {
	return fmt.Sprintf("%s/uploads/%s?sig=%s", strings.TrimRight(l.PublicBase, "/"), key, l.sign(key)), nil
}

// Handler serves PUT /uploads/{key...} after checking the signature.
func (l *Local) Handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		key := strings.TrimPrefix(r.URL.Path, "/uploads/")
		if r.Method != http.MethodPut || strings.Contains(key, "..") || !hmac.Equal([]byte(r.URL.Query().Get("sig")), []byte(l.sign(key))) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		path := filepath.Join(l.Dir, filepath.FromSlash(key))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		f, err := os.Create(path)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer f.Close()
		if _, err := io.Copy(f, r.Body); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}
}
