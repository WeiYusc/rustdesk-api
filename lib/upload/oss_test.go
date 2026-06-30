package upload

import (
	"bytes"
	"io"
	"net/http"
	"testing"
)

func TestValidateOssPublicKeyURLRejectsUnsafeTargets(t *testing.T) {
	cases := []string{
		"http://169.254.169.254/latest/meta-data/",
		"http://127.0.0.1/key",
		"http://[::1]/key",
		"ftp://gosspublic.alicdn.com/key",
		"https://example.com/key",
	}

	for _, raw := range cases {
		t.Run(raw, func(t *testing.T) {
			if _, err := validateOssPublicKeyURL(raw); err == nil {
				t.Fatalf("validateOssPublicKeyURL(%q) succeeded, want rejection", raw)
			}
		})
	}
}

func TestValidateOssPublicKeyURLAllowsAliyunPublicKeyHost(t *testing.T) {
	if _, err := validateOssPublicKeyURL("http://gosspublic.alicdn.com/key"); err != nil {
		t.Fatalf("validateOssPublicKeyURL rejected Aliyun OSS public key host: %v", err)
	}
}

func TestOssPublicKeyHTTPClientDoesNotFollowRedirects(t *testing.T) {
	client := ossPublicKeyHTTPClient()
	if client.CheckRedirect == nil {
		t.Fatalf("oss public key HTTP client follows redirects by default")
	}
	if err := client.CheckRedirect(&http.Request{}, []*http.Request{{}}); err != http.ErrUseLastResponse {
		t.Fatalf("CheckRedirect error = %v, want http.ErrUseLastResponse", err)
	}
}

func TestReadLimitedOssPublicKeyRejectsOversizedBody(t *testing.T) {
	body := bytes.NewBuffer(make([]byte, maxOssPublicKeyBytes+1))
	resp := &http.Response{Body: io.NopCloser(body)}
	defer resp.Body.Close()

	if _, err := readLimitedOssPublicKey(resp); err == nil {
		t.Fatalf("readLimitedOssPublicKey accepted body larger than limit")
	}
}
