package webui

import (
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
)

func TestEmbeddedAssets(t *testing.T) {
	h := Handler()
	index := httptest.NewRecorder()
	h.ServeHTTP(index, httptest.NewRequest("GET", "/", nil))
	paths := regexp.MustCompile(`(?:src|href)="(/assets/[^\"]+)"`).FindAllStringSubmatch(index.Body.String(), -1)
	if len(paths) < 2 {
		t.Fatal("missing bundled script and stylesheet")
	}
	for _, match := range paths {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("GET", match[1], nil))
		if w.Code != 200 || w.Body.Len() == 0 {
			t.Fatalf("asset missing: %s", match[1])
		}
		if strings.HasSuffix(match[1], ".js") && !strings.Contains(w.Header().Get("Content-Type"), "javascript") {
			t.Fatal("incorrect script content type")
		}
	}
}
