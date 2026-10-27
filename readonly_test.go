package airpeek_test

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// TestOnlyReadOperations guards the read-only promise of the whole module.
// Both transports may only send GET: MWAA's InvokeRestApi with
// RestApiMethodGet, and net/http requests built with http.MethodGet. No MWAA
// operation that changes an environment or hands out CLI or web access is
// called anywhere.
func TestOnlyReadOperations(t *testing.T) {
	methods := regexp.MustCompile(`RestApiMethod\w+|http\.Method\w+`)
	allowed := map[string]bool{"RestApiMethodGet": true, "http.MethodGet": true}
	// The method argument of every request built with net/http.
	requests := regexp.MustCompile(`NewRequestWithContext\(\s*[^,]+,\s*([^,]+),|NewRequest\(\s*([^,]+),`)
	forbidden := regexp.MustCompile(`\b(CreateCliToken|CreateWebLoginToken|CreateEnvironment|UpdateEnvironment|DeleteEnvironment|PublishMetrics|TagResource|UntagResource)\b` +
		`|\.(Post|PostForm)\(`)

	err := filepath.WalkDir(".", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return err
		}
		src, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, m := range methods.FindAllString(string(src), -1) {
			if !allowed[m] {
				t.Errorf("%s uses %s; only GET is allowed", path, m)
			}
		}
		for _, m := range requests.FindAllStringSubmatch(string(src), -1) {
			if method := m[1] + m[2]; method != "http.MethodGet" {
				t.Errorf("%s builds a request with method %s; only http.MethodGet is allowed", path, method)
			}
		}
		if m := forbidden.FindString(string(src)); m != "" {
			t.Errorf("%s calls %s", path, m)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
