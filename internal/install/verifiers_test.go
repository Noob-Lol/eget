package install

import (
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestSha256AssetVerifierRejectsInvalidHex(t *testing.T) {
	verifier := &sha256AssetVerifier{
		AssetURL: "https://example.com/tool.sha256",
		Getter: fakeHTTPGetterFunc(func(url string) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader("not-a-sha256")),
			}, nil
		}),
	}

	err := verifier.Verify([]byte("test"))
	if err == nil || !strings.Contains(err.Error(), "invalid checksum") {
		t.Fatalf("expected invalid checksum error, got %v", err)
	}
}
