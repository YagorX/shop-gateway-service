package handlers

import (
	"html/template"
	"net/http"
	"time"
)

type StatusPageData struct {
	ServiceName  string
	Environment  string
	Version      string
	HTTPAddress  string
	HTTPSEnabled bool
	Status       string
	Now          string
}

type StatusHandler struct {
	TemplatePath string
	BaseData     StatusPageData
}

func NewStatusHandler(templatePath string, baseData StatusPageData) *StatusHandler {
	return &StatusHandler{
		TemplatePath: templatePath,
		BaseData:     baseData,
	}
}

func (h *StatusHandler) Page(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if h == nil || h.TemplatePath == "" {
		http.Error(w, "status template is not configured", http.StatusInternalServerError)
		return
	}

	tmpl, err := template.ParseFiles(h.TemplatePath)
	if err != nil {
		http.Error(w, "failed to parse status template", http.StatusInternalServerError)
		return
	}

	data := h.BaseData
	if data.Status == "" {
		data.Status = "ok"
	}
	data.Now = time.Now().Format(time.RFC3339)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")

	if err := tmpl.Execute(w, data); err != nil {
		http.Error(w, "failed to render status page", http.StatusInternalServerError)
		return
	}
}
