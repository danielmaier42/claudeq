package api

import (
	"embed"
	"io/fs"
	"net/http"
	"strings"
)

// The UI is a React app built from web/ by Vite into internal/api/web, and
// embedded here when the daemon is compiled. The build output is not tracked in
// git, only a placeholder that keeps the embed pattern valid; a daemon compiled
// without building the UI first serves notBuiltPage instead of a blank window.
//
//go:embed all:web
var webFS embed.FS

// assetsPrefix is where Vite puts the bundles. Their file names carry a hash of
// their content, so they may be cached for good.
const assetsPrefix = "/assets/"

// webHandler serves the UI from fsys, or from the embedded build when fsys is nil.
func webHandler(fsys fs.FS) http.Handler {
	if fsys == nil {
		fsys, _ = fs.Sub(webFS, "web")
	}
	files := http.FileServer(http.FS(fsys))
	built := false
	if _, err := fs.Stat(fsys, "index.html"); err == nil {
		built = true
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !built && (r.URL.Path == "/" || r.URL.Path == "/index.html") {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(notBuiltPage))
			return
		}
		if strings.HasPrefix(r.URL.Path, assetsPrefix) {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		}
		files.ServeHTTP(w, r)
	})
}

const notBuiltPage = `<!doctype html><html lang="en"><meta charset="utf-8"><title>DispatchQ</title>
<body style="font:14px -apple-system,sans-serif;padding:48px;color:#444">
<h1 style="font-size:18px">DispatchQ: the UI is not built</h1>
<p>This daemon was compiled without its web UI. Run <code>make web</code> (or
<code>cd web &amp;&amp; npm ci &amp;&amp; npm run build</code>) and build the daemon again.</p>
</body></html>`
