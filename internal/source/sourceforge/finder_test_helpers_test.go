package sourceforge

import (
	"io"
	"net/http"
	"strings"
)

type fakeGetter struct {
	responses map[string]string
	requests  []string
}

type fakeHTTPGetterFunc func(url string) (*http.Response, error)

func htmlResponse(body string) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func statusResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}
