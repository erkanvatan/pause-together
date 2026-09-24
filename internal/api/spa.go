package api

import (
	"io"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

const (
	cacheNoCache   = "no-cache"
	cacheImmutable = "public, max-age=31536000, immutable"
)

// spa serves the SvelteKit build. Real files are served as-is; every other path gets index.html,
// so client-side routes like /rooms/{id} load the app. http.FileServer isn't used: it lists
// directories and redirects /index.html.
func spa(build fs.FS) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
			return
		}

		name := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if name == "" || !serveFile(w, r, build, name) {
			if !serveFile(w, r, build, "index.html") {
				http.Error(w, "web build missing", http.StatusInternalServerError)
			}
		}
	})
}

// serveFile serves name if it's a regular file and reports whether it did.
func serveFile(w http.ResponseWriter, r *http.Request, build fs.FS, name string) bool {
	f, err := build.Open(name)
	if err != nil {
		return false
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil || info.IsDir() {
		return false
	}
	content, ok := f.(io.ReadSeeker)
	if !ok {
		return false
	}

	if strings.HasPrefix(name, "_app/immutable/") {
		w.Header().Set("Cache-Control", cacheImmutable)
	} else {
		w.Header().Set("Cache-Control", cacheNoCache)
	}
	http.ServeContent(w, r, name, info.ModTime(), content)
	return true
}
