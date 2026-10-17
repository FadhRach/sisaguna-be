package media

import (
	"context"
	"fmt"
	"mime"
	"os"
	"path"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// Remote adalah salinan cadangan file di object storage. Key sama dengan path lokal: media/<visibility>/<rel>,
// jadi pemulihan penuh cukup menyalin folder bucket kembali ke MEDIA_ROOT.
type Remote interface {
	Put(ctx context.Context, key, file string) error
	Delete(ctx context.Context, key string) error
}

// Key menyusun key object dari visibility dan path relatif.
func Key(v Visibility, rel string) string { return "media/" + string(v) + "/" + rel }

type S3 struct {
	client *s3.Client
	bucket string
}

// R2Endpoint adalah endpoint S3 Cloudflare R2 untuk sebuah akun. Bucket harus privat; tidak ada URL publik R2 yang dibagikan.
func R2Endpoint(accountID string) string {
	return fmt.Sprintf("https://%s.r2.cloudflarestorage.com", accountID)
}

// NewS3 membuat klien untuk endpoint S3-compatible mana pun (dipakai juga di test).
func NewS3(endpoint, keyID, secret, bucket string) *S3 {
	return &S3{bucket: bucket, client: s3.New(s3.Options{
		Region:                     "auto",
		BaseEndpoint:               aws.String(endpoint),
		Credentials:                credentials.NewStaticCredentialsProvider(keyID, secret, ""),
		UsePathStyle:               true,
		RequestChecksumCalculation: aws.RequestChecksumCalculationWhenRequired, // R2 menolak checksum trailer bawaan SDK baru
		ResponseChecksumValidation: aws.ResponseChecksumValidationWhenRequired,
	})}
}

func (r *S3) Put(ctx context.Context, key, file string) error {
	f, err := os.Open(file)
	if err != nil {
		return fmt.Errorf("remote put open: %w", err)
	}
	defer f.Close()
	_, err = r.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket: &r.bucket, Key: &key, Body: f, ContentType: aws.String(mime.TypeByExtension(path.Ext(file))),
	})
	if err != nil {
		return fmt.Errorf("remote put %s: %w", key, err)
	}
	return nil
}

func (r *S3) Delete(ctx context.Context, key string) error {
	if _, err := r.client.DeleteObject(ctx, &s3.DeleteObjectInput{Bucket: &r.bucket, Key: &key}); err != nil {
		return fmt.Errorf("remote delete %s: %w", key, err)
	}
	return nil
}
