package metrics

import (
	"net/http"

	"github.com/gorilla/mux"
	"github.com/iota-uz/iota-sdk/pkg/application"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type PrometheusController struct {
	path string
}

func NewPrometheusController(path string) application.Controller {
	if path == "" {
		path = "/debug/prometheus"
	}
	return &PrometheusController{path: path}
}

func (c *PrometheusController) Key() string {
	return c.path
}

func (c *PrometheusController) Register(r *mux.Router) {
	r.Handle(c.path, promhttp.Handler()).Methods(http.MethodGet)
}
