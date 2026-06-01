package install

import (
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/gookit/goutil/testutil/assert"
)

func TestSelectVerifier(t *testing.T) {
	svc := NewService()
	svc.Sha256VerifierFactory = func(expected string) (Verifier, error) {
		if expected == "bad" {
			return nil, errors.New("bad verifier")
		}
		return &fakeVerifier{name: "verify:" + expected}, nil
	}
	svc.Sha256AssetVerifierFactory = func(assetURL string, opts Options) Verifier {
		_ = opts
		return &fakeVerifier{name: "asset:" + assetURL}
	}
	svc.Sha256PrinterFactory = func() Verifier {
		return &fakeVerifier{name: "printer"}
	}
	svc.NoVerifierFactory = func() Verifier {
		return &fakeVerifier{name: "noop"}
	}

	verifier, err := svc.SelectVerifier("sum.txt", &Options{Verify: "abc"})
	if err != nil {
		t.Fatalf("SelectVerifier(verify): %v", err)
	}
	if got := verifier.(*fakeVerifier).name; got != "verify:abc" {
		t.Fatalf("SelectVerifier(verify) = %q", got)
	}

	verifier, err = svc.SelectVerifier("sum.txt", &Options{})
	if err != nil {
		t.Fatalf("SelectVerifier(asset): %v", err)
	}
	if got := verifier.(*fakeVerifier).name; got != "asset:sum.txt" {
		t.Fatalf("SelectVerifier(asset) = %q", got)
	}

	verifier, err = svc.SelectVerifier("", &Options{Hash: true})
	if err != nil {
		t.Fatalf("SelectVerifier(hash): %v", err)
	}
	if got := verifier.(*fakeVerifier).name; got != "printer" {
		t.Fatalf("SelectVerifier(hash) = %q", got)
	}

	verifier, err = svc.SelectVerifier("", &Options{})
	if err != nil {
		t.Fatalf("SelectVerifier(noop): %v", err)
	}
	if got := verifier.(*fakeVerifier).name; got != "noop" {
		t.Fatalf("SelectVerifier(noop) = %q", got)
	}
}

func TestTemplateChecksumVerifierUsesRenderedManifest(t *testing.T) {
	origHTTPDo := httpDo
	defer func() { httpDo = origHTTPDo }()

	var requestedURL string
	httpDo = func(client *http.Client, req *http.Request) (*http.Response, error) {
		_ = client
		requestedURL = req.URL.String()
		return &http.Response{
			StatusCode: http.StatusOK,
			Status:     "200 OK",
			Body:       io.NopCloser(strings.NewReader(`{"platforms":{"win32-x64":{"checksum":"abc"}}}`)),
		}, nil
	}

	svc := NewService()
	var verifierValue string
	svc.Sha256VerifierFactory = func(expected string) (Verifier, error) {
		verifierValue = expected
		return &fakeVerifier{name: "verify:" + expected}, nil
	}

	verifier, err := svc.SelectVerifier("", &Options{
		URLTemplate: URLTemplateOptions{
			ChecksumURLTemplate: "https://example.com/{version}/manifest.json",
			ChecksumFormat:      "json",
			ChecksumJSONPath:    "platforms.{os}-{arch}.checksum",
			ResolvedVars:        map[string]string{"version": "1.2.3", "os": "win32", "arch": "x64"},
		},
	})
	if err != nil {
		t.Fatalf("SelectVerifier(template checksum): %v", err)
	}

	assert.Eq(t, "verify:abc", verifier.(*fakeVerifier).name)
	assert.Eq(t, "abc", verifierValue)
	assert.Eq(t, "https://example.com/1.2.3/manifest.json", requestedURL)
}

func TestTemplateChecksumVerifierPrefersExplicitVerify(t *testing.T) {
	origHTTPDo := httpDo
	defer func() { httpDo = origHTTPDo }()

	httpDo = func(client *http.Client, req *http.Request) (*http.Response, error) {
		t.Fatalf("manifest should not be requested when verify_sha256 is set")
		return nil, nil
	}

	svc := NewService()
	var verifierValue string
	svc.Sha256VerifierFactory = func(expected string) (Verifier, error) {
		verifierValue = expected
		return &fakeVerifier{name: "verify:" + expected}, nil
	}

	verifier, err := svc.SelectVerifier("", &Options{
		Verify: "explicit",
		URLTemplate: URLTemplateOptions{
			ChecksumURLTemplate: "https://example.com/{version}/manifest.json",
			ChecksumFormat:      "json",
			ChecksumJSONPath:    "platforms.{os}-{arch}.checksum",
			ResolvedVars:        map[string]string{"version": "1.2.3", "os": "win32", "arch": "x64"},
		},
	})
	if err != nil {
		t.Fatalf("SelectVerifier(explicit verify): %v", err)
	}

	assert.Eq(t, "verify:explicit", verifier.(*fakeVerifier).name)
	assert.Eq(t, "explicit", verifierValue)
}
