// SPDX-License-Identifier: AGPL-3.0-only
// Provenance-includes-location: https://github.com/cortexproject/cortex/blob/master/pkg/storage/bucket/sse_bucket_client.go
// Provenance-includes-license: Apache-2.0
// Provenance-includes-copyright: The Cortex Authors.

package objstore

import (
	"context"
	"io"

	"github.com/minio/minio-go/v7/pkg/encrypt"
	"github.com/pkg/errors"
	"github.com/thanos-io/objstore"
	"github.com/thanos-io/objstore/providers/s3"

	phlare_s3 "github.com/grafana/pyroscope/pkg/objstore/providers/s3"
)

// TenantConfigProvider defines a per-tenant config provider.
type TenantConfigProvider interface {
	// S3SSEType returns the per-tenant S3 SSE type.
	S3SSEType(userID string) string

	// S3SSEKMSKeyID returns the per-tenant S3 KMS-SSE key id or an empty string if not set.
	S3SSEKMSKeyID(userID string) string

	// S3SSEKMSEncryptionContext returns the per-tenant S3 KMS-SSE key id or an empty string if not set.
	S3SSEKMSEncryptionContext(userID string) string
}

// SSEBucketClient is a wrapper around a objstore.BucketReader that configures the object
// storage server-side encryption (SSE) for a given user.
type SSEBucketClient struct {
	userID      string
	bucket      Bucket
	cfgProvider TenantConfigProvider
}

// NewSSEBucketClient makes a new SSEBucketClient. The cfgProvider can be nil.
func NewSSEBucketClient(userID string, bucket Bucket, cfgProvider TenantConfigProvider) InstrumentedBucket {
	return &SSEBucketClient{
		userID:      userID,
		bucket:      bucket,
		cfgProvider: cfgProvider,
	}
}

// Close implements objstore.Bucket.
func (b *SSEBucketClient) Close() error {
	return b.bucket.Close()
}

func (b *SSEBucketClient) ReaderAt(ctx context.Context, name string) (ReaderAtCloser, error) {
	return b.bucket.ReaderAt(ctx, name)
}

// Upload the contents of the reader as an object into the bucket.
func (b *SSEBucketClient) Upload(ctx context.Context, name string, r io.Reader) error {
	if sse, err := b.getCustomS3SSEConfig(); err != nil {
		return err
	} else if sse != nil {
		// If the underlying bucket client is not S3 and a custom S3 SSE config has been
		// provided, the config option will be ignored.
		ctx = s3.ContextWithSSEConfig(ctx, sse)
	}

	return b.bucket.Upload(ctx, name, r)
}

// Delete implements objstore.Bucket.
func (b *SSEBucketClient) Delete(ctx context.Context, name string) error {
	return b.bucket.Delete(ctx, name)
}

// Name implements objstore.Bucket.
func (b *SSEBucketClient) Name() string {
	return b.bucket.Name()
}

func (b *SSEBucketClient) getCustomS3SSEConfig() (encrypt.ServerSide, error) {
	if b.cfgProvider == nil {
		return nil, nil
	}

	// No S3 SSE override if the type override hasn't been provided.
	sseType := b.cfgProvider.S3SSEType(b.userID)
	if sseType == "" {
		return nil, nil
	}

	cfg := phlare_s3.SSEConfig{
		Type:                 sseType,
		KMSKeyID:             b.cfgProvider.S3SSEKMSKeyID(b.userID),
		KMSEncryptionContext: b.cfgProvider.S3SSEKMSEncryptionContext(b.userID),
	}

	sse, err := cfg.BuildMinioConfig()
	if err != nil {
		return nil, errors.Wrapf(err, "unable to customise S3 SSE config for tenant %s", b.userID)
	}

	return sse, nil
}

// Iter implements objstore.Bucket.
func (b *SSEBucketClient) Iter(ctx context.Context, dir string, f func(string) error, options ...objstore.IterOption) error {
	return b.bucket.Iter(ctx, dir, f, options...)
}

func (b *SSEBucketClient) IterWithAttributes(ctx context.Context, dir string, f func(attrs objstore.IterObjectAttributes) error, options ...objstore.IterOption) error {
	return b.bucket.IterWithAttributes(ctx, dir, f, options...)
}

func (b *SSEBucketClient) SupportedIterOptions() []objstore.IterOptionType {
	return b.bucket.SupportedIterOptions()
}

// Get implements objstore.Bucket.
func (b *SSEBucketClient) Get(ctx context.Context, name string) (io.ReadCloser, error) {
	return b.bucket.Get(ctx, name)
}

// GetRange implements objstore.Bucket.
func (b *SSEBucketClient) GetRange(ctx context.Context, name string, off, length int64) (io.ReadCloser, error) {
	return b.bucket.GetRange(ctx, name, off, length)
}

// Exists implements objstore.Bucket.
func (b *SSEBucketClient) Exists(ctx context.Context, name string) (bool, error) {
	return b.bucket.Exists(ctx, name)
}

// IsObjNotFoundErr implements objstore.Bucket.
func (b *SSEBucketClient) IsObjNotFoundErr(err error) bool {
	return b.bucket.IsObjNotFoundErr(err)
}

// IsAccessDeniedErr returns true if acces to object is denied.
func (b *SSEBucketClient) IsAccessDeniedErr(err error) bool {
	return b.bucket.IsAccessDeniedErr(err)
}

// Attributes implements objstore.Bucket.
func (b *SSEBucketClient) Attributes(ctx context.Context, name string) (objstore.ObjectAttributes, error) {
	return b.bucket.Attributes(ctx, name)
}

// ReaderWithExpectedErrs implements objstore.Bucket.
func (b *SSEBucketClient) ReaderWithExpectedErrs(fn IsOpFailureExpectedFunc) BucketReader {
	return b.WithExpectedErrs(fn)
}

// WithExpectedErrs implements objstore.Bucket.
func (b *SSEBucketClient) WithExpectedErrs(fn IsOpFailureExpectedFunc) Bucket {
	if ib, ok := b.bucket.(InstrumentedBucket); ok {
		return &SSEBucketClient{
			userID:      b.userID,
			bucket:      ib.WithExpectedErrs(fn),
			cfgProvider: b.cfgProvider,
		}
	}

	return b
}

func (b *SSEBucketClient) Provider() objstore.ObjProvider {
	return b.bucket.Provider()
}
