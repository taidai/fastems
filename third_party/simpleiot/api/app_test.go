package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"
)

func TestFontsAreCacheable(t *testing.T) {
	fsys := fstest.MapFS{
		"index.html":   {Data: []byte("<html></html>")},
		"fonts/a.css":  {Data: []byte("@font-face{}")},
		"fonts/a.woff": {Data: []byte("x")},
	}
	app := &App{PublicHandler: http.FileServer(http.FS(fsys))}

	for path, want := range map[string]string{
		"/fonts/a.css":  "public, max-age=31536000, immutable",
		"/fonts/a.woff": "public, max-age=31536000, immutable",
		"/old/":         "",
	} {
		rec := httptest.NewRecorder()
		app.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != http.StatusOK {
			t.Errorf("%v: status %v", path, rec.Code)
		}
		if got := rec.Header().Get("Cache-Control"); got != want {
			t.Errorf("%v: Cache-Control %q, want %q", path, got, want)
		}
	}
}

func TestAppOnlyRoutes(t *testing.T) {
	app := NewAppHandler(ServerArgs{
		AppOnly:      true,
		UIFilesystem: fstest.MapFS{"index.html": {Data: []byte("easyems")}},
		AppHandler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(r.URL.Path))
		}),
	})
	for _, path := range []string{"/v1/nodes", "/old", "/old/sign-in", "/fonts/a.css"} {
		rec := httptest.NewRecorder()
		app.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != http.StatusNotFound {
			t.Errorf("%s: status %d, want 404", path, rec.Code)
		}
	}
	for path, want := range map[string]string{"/": "easyems", "/api/health": "/health"} {
		rec := httptest.NewRecorder()
		app.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != http.StatusOK || rec.Body.String() != want {
			t.Errorf("%s: status %d body %q", path, rec.Code, rec.Body.String())
		}
	}
}
