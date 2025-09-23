package s3

import (
	"github.com/arencloud/hermes/internal/models"
	"testing"
)

func TestNormalizeEndpoint(t *testing.T) {
	cases := []struct {
		in     string
		ssl    bool
		host   string
		secure bool
	}{
		{"minio.local:9000", false, "minio.local:9000", false},
		{"http://minio.local:9000", true, "minio.local:9000", false},
		{"https://s3.amazonaws.com", false, "s3.amazonaws.com", true},
	}
	for _, c := range cases {
		h, sec := normalizeEndpoint(c.in, c.ssl)
		if h != c.host || sec != c.secure {
			t.Fatalf("normalizeEndpoint(%q,%v)=%q,%v want %q,%v", c.in, c.ssl, h, sec, c.host, c.secure)
		}
	}
}

func TestForcePathStyle(t *testing.T) {
	if !forcePathStyle(models.Provider{Type: "minio"}) {
		t.Fatal("minio should be path-style")
	}
	if !forcePathStyle(models.Provider{Type: "generic"}) {
		t.Fatal("generic should be path-style")
	}
	if forcePathStyle(models.Provider{Type: "aws"}) {
		t.Fatal("aws should not force path-style")
	}
}
