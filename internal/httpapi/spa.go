package httpapi

import (
	"io/fs"
	"net/http"
	"path"
	"strings"
)

// noFrontendPage answers when the binary was built without the web app, which
// is the normal state while running the Vite dev server.
const noFrontendPage = `<!doctype html><meta charset="utf-8"><title>ownership_board</title>
<body style="font-family:system-ui;background:#0e0f13;color:#e7e9ee;padding:2rem">
<p>The web app is not embedded in this build. During development run <code>npm run dev</code> in <code>web/</code>
and open the Vite URL. Release builds embed it with <code>-tags embed_frontend</code>.</p>`

// spa serves the built web app. Hashed assets are cached forever; index.html
// and the service worker are never cached, so a deploy shows up immediately.
type spa struct {
	files fs.FS
	fs    http.Handler
}

func newSPA(files fs.FS) http.Handler {
	if files == nil {
		return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte(noFrontendPage))
		})
	}
	return spa{files: files, fs: http.FileServerFS(files)}
}

func (s spa) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
	if name == "" || !s.exists(name) {
		s.index(w, r)
		return
	}
	if strings.HasPrefix(name, "assets/") {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	} else {
		w.Header().Set("Cache-Control", "no-cache")
	}
	s.fs.ServeHTTP(w, r)
}

// index serves the app shell for any client-side route.
func (s spa) index(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-cache")
	http.ServeFileFS(w, r, s.files, "index.html")
}

func (s spa) exists(name string) bool {
	info, err := fs.Stat(s.files, name)
	return err == nil && !info.IsDir()
}

// readOnly refuses anything but reads, since the web app is static files.
func readOnly(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		next.ServeHTTP(w, r)
	})
}
