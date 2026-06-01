package client

import (
	"bytes"
	"io"
	"net/http"
)

func jsonResponse(code int, status, body string) *http.Response {
	return &http.Response{
		StatusCode: code,
		Status:     status,
		Body:       io.NopCloser(bytes.NewBufferString(body)),
		Header:     make(http.Header),
	}
}
