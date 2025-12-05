package controllers

import (
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"path"
	"path/filepath"

	"github.com/a-h/templ"
	"github.com/gorilla/mux"

	"github.com/iota-uz/iota-sdk/components"
	"github.com/iota-uz/iota-sdk/modules/core/domain/entities/permission"
	"github.com/iota-uz/iota-sdk/modules/core/domain/entities/upload"
	"github.com/iota-uz/iota-sdk/modules/core/permissions"
	"github.com/iota-uz/iota-sdk/modules/core/presentation/mappers"
	"github.com/iota-uz/iota-sdk/modules/core/services"
	"github.com/iota-uz/iota-sdk/pkg/application"
	"github.com/iota-uz/iota-sdk/pkg/authz"
	"github.com/iota-uz/iota-sdk/pkg/configuration"
	"github.com/iota-uz/iota-sdk/pkg/mapping"
	"github.com/iota-uz/iota-sdk/pkg/middleware"
)

type UploadController struct {
	app           application.Application
	uploadService *services.UploadService
	basePath      string
}

func ensureUploadsAuthz(w http.ResponseWriter, r *http.Request, action string) bool {
	return ensureAuthz(w, r, uploadsAuthzObject, action, legacyUploadPermission(action))
}

func legacyUploadPermission(action string) *permission.Permission {
	switch action {
	case "list", "view":
		return permissions.UploadRead
	case "create":
		return permissions.UploadCreate
	case "update":
		return permissions.UploadUpdate
	case "delete":
		return permissions.UploadDelete
	default:
		return nil
	}
}

var uploadsAuthzObject = authz.ObjectName("core", "uploads")

func NewUploadController(app application.Application) application.Controller {
	return &UploadController{
		app:           app,
		uploadService: app.Service(services.UploadService{}).(*services.UploadService),
		basePath:      "/uploads",
	}
}

func (c *UploadController) Key() string {
	return "/upload"
}

func (c *UploadController) Register(r *mux.Router) {
	conf := configuration.Use()
	router := r.PathPrefix(c.basePath).Subrouter()
	router.Use(
		middleware.Authorize(),
		middleware.RequireAuthorization(),
		middleware.ProvideUser(),
	)
	router.Use(middleware.ProvideLocalizer(c.app))
	router.Use(middleware.WithTransaction())
	router.HandleFunc("", c.Create).Methods(http.MethodPost)

	workDir, err := os.Getwd()
	if err != nil {
		panic(err)
	}
	fullPath := filepath.Join(workDir, conf.UploadsPath)
	prefix := path.Join("/", conf.UploadsPath, "/")
	r.PathPrefix(prefix).Handler(http.StripPrefix(prefix, http.FileServer(http.Dir(fullPath))))
}

func (c *UploadController) Create(w http.ResponseWriter, r *http.Request) {
	if !ensureUploadsAuthz(w, r, "create") {
		return
	}
	conf := configuration.Use()
	if err := r.ParseMultipartForm(conf.MaxUploadMemory); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	files, ok := r.MultipartForm.File["file"]
	if !ok {
		http.Error(w, "No file(s) found", http.StatusBadRequest)
		return
	}

	id := r.FormValue("_id")
	name := r.FormValue("_name")
	formName := r.FormValue("_formName")
	multiple := r.FormValue("_multiple") == "true"

	dtos := make([]*upload.CreateDTO, 0, len(files))
	for _, header := range files {
		file, err := header.Open()
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		defer func(file multipart.File) {
			if err := file.Close(); err != nil {
				log.Println(err)
			}
		}(file)

		dto := &upload.CreateDTO{
			File: file,
			Name: header.Filename,
			Size: int(header.Size),
		}

		// TODO: proper error handling
		if _, ok := dto.Ok(r.Context()); !ok {
			_, _, err := dto.ToEntity()
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			props := &components.UploadInputProps{
				ID:       id,
				Uploads:  nil,
				Error:    "",
				Name:     name,
				Form:     formName,
				Multiple: multiple,
			}
			templ.Handler(components.UploadTarget(props), templ.WithStreaming()).ServeHTTP(w, r)
			return
		}
		dtos = append(dtos, dto)
	}

	uploadEntities, err := c.uploadService.CreateMany(r.Context(), dtos)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	props := &components.UploadInputProps{
		ID:       id,
		Uploads:  mapping.MapViewModels(uploadEntities, mappers.UploadToViewModel),
		Name:     name,
		Form:     formName,
		Multiple: multiple,
	}

	templ.Handler(components.UploadTarget(props), templ.WithStreaming()).ServeHTTP(w, r)
}
