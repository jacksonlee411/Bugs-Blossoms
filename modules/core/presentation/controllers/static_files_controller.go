package controllers

import (
	"net/http"

	"github.com/benbjohnson/hashfs"
	"github.com/gorilla/mux"
	"github.com/iota-uz/iota-sdk/pkg/application"
	"github.com/iota-uz/iota-sdk/pkg/configuration"
	"github.com/iota-uz/iota-sdk/pkg/multifs"
)

type StaticFilesController struct {
	fsInstances []*hashfs.FS
}

func (s *StaticFilesController) Key() string {
	return "/assets"
}

func (s *StaticFilesController) Register(r *mux.Router) {
	fsHandler := http.StripPrefix("/assets/", http.FileServer(multifs.New(s.fsInstances...)))
	cacheControl := "public, max-age=3600"
	if configuration.Use().GoAppEnvironment != configuration.Production {
		cacheControl = "no-cache, no-store, must-revalidate"
	}
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", cacheControl)
		if cacheControl != "public, max-age=3600" {
			w.Header().Set("Pragma", "no-cache")
			w.Header().Set("Expires", "0")
		}
		fsHandler.ServeHTTP(w, r)
	})
	r.PathPrefix("/assets/").Handler(handler)
}

func NewStaticFilesController(fsInstances []*hashfs.FS) application.Controller {
	return &StaticFilesController{
		fsInstances: fsInstances,
	}
}
