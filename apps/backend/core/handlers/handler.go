package handlers

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/go-chi/chi/v5"
	// "github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"
	"github.com/richd0tcom/piped/config"
	"github.com/richd0tcom/piped/core/server"
	"github.com/richd0tcom/piped/internal/models"
)

type Handler struct {
	srv *server.Server
}

func NewDeploymentHandler(srv *server.Server) *Handler {
	return &Handler{srv: srv}
}

func (h *Handler) ListDeployments(w http.ResponseWriter, r *http.Request) {

	ctx := r.Context()
	deployments, err := h.srv.Store.ListDeployments(ctx)
	if err != nil {
		jsonErr(w, err, 500)
		return
	}
	jsonOK(w, deployments)
}

func (a *Handler) GetDeployment(w http.ResponseWriter, r *http.Request) {
	ctx:= r.Context()
	d, err := a.srv.Store.GetDeployment(ctx, chi.URLParam(r, "id"))
	if err != nil {
		jsonErr(w, err, 404)
		return
	}
	jsonOK(w, d)
}

type createRequest struct {
	Name      string            `json:"name"`
	SourceType models.SourceType`json:"source_type"` // git | upload
	GitURL    string            `json:"git_url"`
	GitCommit string            `json:"git_commit"`
	EnvVars   map[string]string `json:"env_vars"`
}

func (a *Handler) CreateDeployment(w http.ResponseWriter, r *http.Request) {

	ctx:= r.Context()
	// multipart for uploads, JSON for git
	contentType := r.Header.Get("Content-Type")

	var req createRequest
	var archivePath string

	if len(contentType) > 19 && contentType[:19] == "multipart/form-data" {
		r.ParseMultipartForm(64 << 20) // 64mb
		req.Name = r.FormValue("name")
		req.SourceType = models.SourceUpload
		req.EnvVars = parseEnvVars(r.FormValue("env_vars"))

		file, header, err := r.FormFile("archive")
		if err != nil {
			jsonErr(w, fmt.Errorf("archive required"), 400)
			return
		}
		defer file.Close()

		//TODO: load upload dir from env or constant
		archivePath = filepath.Join(a.srv.Config.GetString(config.EnvUploadDir), uuid.New().String()+"-"+header.Filename)
		out, err := os.Create(archivePath)
		if err != nil {
			jsonErr(w, err, 500)
			return
		}
		defer out.Close()
		io.Copy(out, file)
	} else {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			jsonErr(w, err, 400)
			return
		}
	}

	if req.Name == "" {
		jsonErr(w, fmt.Errorf("name required"), 400)
		return
	}
	if req.SourceType == models.SourceGit && req.GitURL == "" {
		jsonErr(w, fmt.Errorf("git_url required"), 400)
		return
	}

	now := time.Now()
	d := &models.Deployment{
		ID:         uuid.New().String(), //TODO: change to base64char
		Name:       req.Name,
		SourceType: req.SourceType,
		ResourceURL:     req.GitURL,
		GitCommit:  req.GitCommit,
		EnvVars:    req.EnvVars,
		Status:     models.StatusPending,
		CreatedAt:  now,
		UpdatedAt:  now,
	}

	if req.SourceType == models.SourceUpload {
		d.ResourceURL = archivePath // reuse field for archive path
	}

	if err := a.srv.Store.CreateDeployment(ctx, d); err != nil {
		jsonErr(w, err, 500)
		return
	}

	a.srv.Maestro.Deploy(d.ID)

	w.WriteHeader(http.StatusAccepted)
	jsonOK(w, d)
}

func (a *Handler) DeleteDeployment(w http.ResponseWriter, r *http.Request) {
	if err := a.srv.Maestro.Teardown(chi.URLParam(r, "id")); err != nil {
		jsonErr(w, err, 500)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *Handler) RedeployDeployment(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	a.srv.Maestro.Deploy(id)
	w.WriteHeader(http.StatusAccepted)
}

func (a *Handler) RestartDeployment(w http.ResponseWriter, r *http.Request) {
	if err := a.srv.Maestro.Restart(chi.URLParam(r, "id")); err != nil {
		jsonErr(w, err, 500)
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

type rollbackRequest struct {
	ImageTag string `json:"image_tag"`
}

func (a *Handler) RollbackDeployment(w http.ResponseWriter, r *http.Request) {
	var req rollbackRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		jsonErr(w, err, 400)
		return
	}
	if req.ImageTag == "" {
		jsonErr(w, fmt.Errorf("image_tag required"), 400)
		return
	}
	if err := a.srv.Maestro.Rollback(chi.URLParam(r, "id"), req.ImageTag); err != nil {
		jsonErr(w, err, 500)
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

// streamLogs handles SSE for a deployment.
// It replays history first, then streams live lines.
func (a *Handler) StreamLogs(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")


	ctx := r.Context()

	// verify deployment exists
	//TODO: cross check this to avoid memory/state leaks and other issues
	if _, err := a.srv.Store.GetDeployment(ctx, id); err != nil {
		jsonErr(w, err, 404)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no") // disable nginx/caddy buffering

	flusher, ok := w.(http.Flusher)
	if !ok {
		jsonErr(w, fmt.Errorf("streaming unsupported"), 500)
		return
	}

	// 1. replay history
	history, err := a.srv.Store.GetLogs(ctx, id, 0)
	if err == nil {
		for _, line := range history {
			writeSSE(w, line)
		}
		flusher.Flush()
	}

	// 2. subscribe to live lines
	ch := a.srv.Portal.Subscribe(id)
	defer a.srv.Portal.Unsubscribe(id, ch)

	// keep-alive ticker so the connection stays open
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case line, ok := <-ch:
			if !ok {
				return
			}
			writeSSE(w, line)
			flusher.Flush()
		case <-ticker.C:
			fmt.Fprintf(w, ": ping\n\n")
			flusher.Flush()
		case <-r.Context().Done():
			return
		}
	}
}

// StreamStatus handles SSE for deployment status updates.
// Streams status changes in real-time.
func (a *Handler) StreamStatus(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")

	ctx := r.Context()

	// verify deployment exists
	if _, err := a.srv.Store.GetDeployment(ctx, id); err != nil {
		jsonErr(w, err, 404)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	flusher, ok := w.(http.Flusher)
	if !ok {
		jsonErr(w, fmt.Errorf("streaming unsupported"), 500)
		return
	}

	// Send current status first
	d, err := a.srv.Store.GetDeployment(ctx, id)
	if err == nil {
		writeStatusSSE(w, string(d.Status))
		flusher.Flush()
	}

	// Subscribe to live status updates
	ch := a.srv.Portal.SubscribeStatus(id)
	defer a.srv.Portal.UnsubscribeStatus(id, ch)

	// keep-alive ticker
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case status, ok := <-ch:
			if !ok {
				return
			}
			writeStatusSSE(w, status)
			flusher.Flush()
		case <-ticker.C:
			fmt.Fprintf(w, ": ping\n\n")
			flusher.Flush()
		case <-r.Context().Done():
			return
		}
	}
}

func writeStatusSSE(w http.ResponseWriter, status string) {
	data, _ := json.Marshal(map[string]string{"status": status})
	fmt.Fprintf(w, "data: %s\n\n", data)
}

// --- SSE ---

func writeSSE(w http.ResponseWriter, line *models.LogLine) {
	data, _ := json.Marshal(line)
	fmt.Fprintf(w, "data: %s\n\n", data)
}

// --- helpers ---

func jsonOK(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

func jsonErr(w http.ResponseWriter, err error, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func parseEnvVars(raw string) map[string]string {
	out := map[string]string{}
	if raw == "" {
		return out
	}
	json.Unmarshal([]byte(raw), &out)
	return out
}