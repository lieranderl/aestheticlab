package server_test

import (
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/lieranderl/aestheticlab/internal/server"
)

// gunzip returns the decompressed body of a gzip response.
func gunzip(t *testing.T, res *httptest.ResponseRecorder) string {
	t.Helper()
	zr, err := gzip.NewReader(res.Body)
	if err != nil {
		t.Fatalf("the body is not gzip: %v", err)
	}
	body, err := io.ReadAll(zr)
	if err != nil {
		t.Fatalf("reading the gzip body: %v", err)
	}
	return string(body)
}

// Pages are rendered per request, so they are compressed on the fly; the
// pricelist is 250 KB of HTML and about 55 KB gzipped.
func TestPagesAreGzippedForBrowsersThatAcceptIt(t *testing.T) {
	h := server.New(server.Config{})
	for _, path := range []string{"/en-BE/", "/en-BE/pricelist/", "/en-BE/missing-page/"} {
		t.Run(path, func(t *testing.T) {
			res := getWithEncoding(t, h, path, "gzip, deflate, br, zstd")
			if got := res.Header().Get("Content-Encoding"); got != "gzip" {
				t.Fatalf("Content-Encoding = %q, want gzip", got)
			}
			if got := res.Header().Get("Vary"); !strings.Contains(got, "Accept-Encoding") {
				t.Errorf("Vary = %q, want Accept-Encoding: caches must keep each encoding apart", got)
			}
			if got := res.Header().Get("Content-Type"); got != "text/html; charset=utf-8" {
				t.Errorf("Content-Type = %q, want the page's own type", got)
			}
			if body := gunzip(t, res); !strings.Contains(body, `<html lang="en-BE">`) {
				t.Errorf("the decompressed body is not the page: %.200s", body)
			}
		})
	}
}

func TestPagesAreSentAsTheyAreWithoutGzip(t *testing.T) {
	h := server.New(server.Config{})
	for _, accept := range []string{"", "identity", "br", "gzip;q=0"} {
		t.Run(accept, func(t *testing.T) {
			res := getWithEncoding(t, h, "/en-BE/pricelist/", accept)
			if got := res.Header().Get("Content-Encoding"); got != "" {
				t.Errorf("Content-Encoding = %q, want none", got)
			}
			if !strings.Contains(res.Body.String(), `id="pricelist-tabs"`) {
				t.Error("the body is not the pricelist")
			}
		})
	}
}

func TestADatastarPatchIsGzippedToo(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/en-BE/?treatment=laser", nil)
	req.Header.Set("Datastar-Request", "true")
	req.Header.Set("Accept-Encoding", "gzip")
	res := serve(t, server.New(server.Config{}), req)
	if got := res.Header().Get("Content-Encoding"); got != "gzip" {
		t.Fatalf("Content-Encoding = %q, want gzip", got)
	}
	if body := gunzip(t, res); !strings.Contains(body, "event: datastar-patch-elements") {
		t.Errorf("the decompressed body is not a patch: %.200s", body)
	}
}

// Images, fonts and video are compressed already; gzipping them again only
// costs CPU and breaks range requests.
func TestMediaIsNeverGzippedOnTheFly(t *testing.T) {
	res := getWithEncoding(t, compressedSite(), "/static/dist/photo.3f2a9c1b00.webp", "gzip")
	if got := res.Header().Get("Content-Encoding"); got != "" {
		t.Errorf("Content-Encoding = %q, want none", got)
	}
}

// Static files choose their own precompressed representation and say so
// once; the page compressor stays out of their way.
func TestStaticFilesVaryOnAcceptEncodingOnce(t *testing.T) {
	for _, path := range []string{"/static/dist/app.3f2a9c1b00.css", "/static/dist/photo.3f2a9c1b00.webp"} {
		res := getWithEncoding(t, compressedSite(), path, "gzip, br")
		if got := res.Header().Values("Vary"); len(got) != 1 || got[0] != "Accept-Encoding" {
			t.Errorf("%s: Vary = %q, want one Accept-Encoding", path, got)
		}
	}
}
