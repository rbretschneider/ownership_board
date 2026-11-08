package httpapi

import (
	"github.com/rbretschneider/ownership_board/internal/install"
	"net/http"
	"strconv"
)

func (s *server) installRoutes(rt *router) {
	for _, kind := range []string{"sh", "ps1"} {
		rt.public("GET /install/build-host."+kind, func(w http.ResponseWriter, r *http.Request, _ caller) error {
			raw, err := install.Render(kind)
			if err != nil {
				return err
			}
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			w.Header().Set("Content-Disposition", `attachment; filename="build-host.`+kind+`"`)
			w.Header().Set("Cache-Control", "no-store")
			w.Header().Set("Content-Length", strconv.Itoa(len(raw)))
			if r.Method != "HEAD" {
				_, err = w.Write(raw)
			}
			return err
		})
	}
}
