package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/NoelOrin/objectstore"

	"GOSpeak/internal/model"
)

// ObjectStoreProvider 用 objectstore.Backend 承载对象读写；
// 预签名直传与公开 URL 属于调用方语义，仍由本层提供。
type ObjectStoreProvider struct {
	backend     objectstore.Backend
	presigner   objectstore.Presigner
	publicURLer objectstore.PublicURLer
}

var _ Provider = (*ObjectStoreProvider)(nil)

// NewProvider 按存储配置构造 provider。
func NewProvider(cfg model.StorageConfig, localDir string) (Provider, error) {
	libCfg := objectstore.Config{
		LocalDir:        localDir,
		S3Endpoint:      cfg.Endpoint,
		S3Bucket:        cfg.Bucket,
		S3Region:        cfg.Region,
		S3AccessKey:     cfg.AccessKey,
		S3SecretKey:     cfg.SecretKey,
		S3PublicBaseURL: cfg.PublicBaseURL,
		S3Timeout:       5 * time.Second,
		S3PresignTTL:    5 * time.Minute,
		// handler 已把 path_prefix 写进 object key，这里不能再设 S3Prefix。
	}
	if cfg.PublicBaseURL != "" {
		libCfg.PublicURL = cfg.PublicBaseURL
	} else {
		libCfg.PublicURL = "/uploads"
	}
	switch cfg.ProviderType {
	case "s3":
		libCfg.Driver = "s3"
	case "local", "":
		libCfg.Driver = "local"
		if libCfg.LocalDir == "" {
			libCfg.LocalDir = "./uploads"
		}
	default:
		return nil, fmt.Errorf("provider_type must be s3 or local")
	}

	backend, err := objectstore.Open(libCfg)
	if err != nil {
		return nil, err
	}
	p := &ObjectStoreProvider{backend: backend}
	if presigner, ok := backend.(objectstore.Presigner); ok {
		p.presigner = presigner
	}
	if publicURLer, ok := backend.(objectstore.PublicURLer); ok {
		p.publicURLer = publicURLer
	}
	return p, nil
}

func (p *ObjectStoreProvider) PresignUpload(key string, contentType string, size int64) (*PresignedResult, error) {
	if p.presigner == nil {
		return &PresignedResult{ObjectKey: key}, nil
	}
	up, err := p.presigner.PresignUpload(context.Background(), key, contentType, size, 0)
	if err != nil {
		return nil, err
	}
	return &PresignedResult{
		UploadURL: up.URL,
		ObjectKey: up.ObjectKey,
		PublicURL: p.GetPublicURL(up.ObjectKey),
	}, nil
}

func (p *ObjectStoreProvider) UploadFromReader(key string, reader io.Reader, size int64, contentType string) (string, error) {
	var src io.Reader = reader
	if size > 0 {
		src = io.LimitReader(reader, size+1)
	}
	data, err := io.ReadAll(src)
	if err != nil {
		return "", fmt.Errorf("read upload failed: %w", err)
	}
	if size > 0 && int64(len(data)) != size {
		return "", fmt.Errorf("upload truncated: read %d of %d bytes", len(data), size)
	}
	if err := p.backend.Put(context.Background(), key, data, contentType); err != nil {
		return "", err
	}
	return p.GetPublicURL(key), nil
}

func (p *ObjectStoreProvider) GetPublicURL(key string) string {
	if p.publicURLer == nil {
		return key
	}
	return p.publicURLer.PublicURL(key)
}

func (p *ObjectStoreProvider) Delete(key string) error {
	return p.backend.Delete(context.Background(), key)
}

func (p *ObjectStoreProvider) TestConnection() error {
	return p.backend.TestConnection(context.Background())
}

func (p *ObjectStoreProvider) HeadObjectSize(key string) (int64, error) {
	size, _, err := p.backend.Stat(context.Background(), key)
	if errors.Is(err, objectstore.ErrNotFound) {
		return 0, fmt.Errorf("object not found: %s", key)
	}
	if err != nil {
		return 0, fmt.Errorf("stat object failed: %w", err)
	}
	return size, nil
}
