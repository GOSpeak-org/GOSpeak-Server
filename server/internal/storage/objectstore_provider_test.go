package storage

import (
	"path/filepath"
	"strings"
	"testing"

	"GOSpeak/internal/model"
)

func newLocalProvider(t *testing.T, publicBaseURL string) Provider {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "uploads")
	p, err := NewProvider(model.StorageConfig{ProviderType: "local", PublicBaseURL: publicBaseURL}, dir)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestProviderLocalRoundTrip(t *testing.T) {
	p := newLocalProvider(t, "")
	key := "user-1/avatar/a.png"
	url, err := p.UploadFromReader(key, strings.NewReader("hello"), 5, "image/png")
	if err != nil {
		t.Fatal(err)
	}
	if url != "/uploads/"+key {
		t.Fatalf("public URL = %q", url)
	}
	size, err := p.HeadObjectSize(key)
	if err != nil || size != 5 {
		t.Fatalf("HeadObjectSize = %d, %v", size, err)
	}
	if err := p.Delete(key); err != nil {
		t.Fatal(err)
	}
	if _, err := p.HeadObjectSize(key); err == nil || !strings.Contains(err.Error(), "object not found") {
		t.Fatalf("deleted object error = %v", err)
	}
}

func TestProviderRejectsTruncatedUpload(t *testing.T) {
	p := newLocalProvider(t, "")
	key := "user-1/avatar/b.png"
	if _, err := p.UploadFromReader(key, strings.NewReader("abc"), 5, "image/png"); err == nil {
		t.Fatal("truncated upload must fail")
	}
	if _, err := p.HeadObjectSize(key); err == nil {
		t.Fatal("truncated upload must not leave an object")
	}
}

func TestProviderLocalPresignHasNoUploadURL(t *testing.T) {
	p := newLocalProvider(t, "")
	res, err := p.PresignUpload("user-1/avatar/c.png", "image/png", 3)
	if err != nil {
		t.Fatal(err)
	}
	if res.UploadURL != "" || res.ObjectKey == "" {
		t.Fatalf("unexpected local presign: %+v", res)
	}
}

func TestProviderPublicBaseURL(t *testing.T) {
	p := newLocalProvider(t, "https://cdn.example.com/")
	if got := p.GetPublicURL("a/b.png"); got != "https://cdn.example.com/a/b.png" {
		t.Fatalf("public URL = %q", got)
	}
}

func TestProviderUnknownDriver(t *testing.T) {
	if _, err := NewProvider(model.StorageConfig{ProviderType: "oss"}, "uploads"); err == nil {
		t.Fatal("unknown provider type must fail")
	}
}

func TestProviderS3PresignAndPublicURL(t *testing.T) {
	p, err := NewProvider(model.StorageConfig{
		ProviderType: "s3",
		Endpoint:     "https://s3.example.test",
		Bucket:       "bucket",
		Region:       "us-east-1",
		AccessKey:    "AKIDEXAMPLE",
		SecretKey:    "secret",
	}, "")
	if err != nil {
		t.Fatal(err)
	}
	res, err := p.PresignUpload("user-1/a.png", "image/png", 3)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.UploadURL, "X-Amz-Signature") {
		t.Fatalf("missing signature: %s", res.UploadURL)
	}
	if res.PublicURL != "https://s3.example.test/bucket/user-1/a.png" {
		t.Fatalf("public URL = %q", res.PublicURL)
	}
}
