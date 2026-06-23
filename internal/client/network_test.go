package client

import (
	"net/http"
	"strings"
	"testing"

	"github.com/gookit/goutil/x/assert"
)

func TestGetWithOptionsSetsDefaultUserAgent(t *testing.T) {
	var gotUA string
	restoreHTTPDo := SetHTTPDoForTest(func(client *http.Client, req *http.Request) (*http.Response, error) {
		gotUA = req.Header.Get("User-Agent")
		return jsonResponse(http.StatusOK, "200 OK", `<html></html>`), nil
	})
	defer restoreHTTPDo()

	resp, err := GetWithOptions("https://example.com/tool.zip", Options{})
	if err != nil {
		t.Fatalf("GetWithOptions(): %v", err)
	}
	defer resp.Body.Close()

	assert.Eq(t, DefaultUserAgent, gotUA)
	assert.False(t, strings.Contains(gotUA, "Go-http-client"))
}

func TestGetWithOptionsKeepsSourceForgeDownloadUserAgentUnset(t *testing.T) {
	var gotUA string
	restoreHTTPDo := SetHTTPDoForTest(func(client *http.Client, req *http.Request) (*http.Response, error) {
		gotUA = req.Header.Get("User-Agent")
		return jsonResponse(http.StatusOK, "200 OK", `zip body`), nil
	})
	defer restoreHTTPDo()

	resp, err := GetWithOptions("https://downloads.sourceforge.net/project/victoria-ssd-hdd/Victoria537.zip", Options{})
	if err != nil {
		t.Fatalf("GetWithOptions(): %v", err)
	}
	defer resp.Body.Close()

	assert.Eq(t, "", gotUA)
}

func TestGetWithOptionsUsesConfiguredUserAgent(t *testing.T) {
	var gotUA string
	restoreHTTPDo := SetHTTPDoForTest(func(client *http.Client, req *http.Request) (*http.Response, error) {
		gotUA = req.Header.Get("User-Agent")
		return jsonResponse(http.StatusOK, "200 OK", `<html></html>`), nil
	})
	defer restoreHTTPDo()

	resp, err := GetWithOptions("https://sourceforge.net/projects/victoria-ssd-hdd/files/", Options{UserAgent: "custom-agent/1.0"})
	if err != nil {
		t.Fatalf("GetWithOptions(): %v", err)
	}
	defer resp.Body.Close()

	assert.Eq(t, "custom-agent/1.0", gotUA)
}
