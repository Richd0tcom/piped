package routing

import (

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/richd0tcom/piped/core/handlers"
	"github.com/richd0tcom/piped/core/server"
)




func SetupRouter(srv *server.Server) {
	
	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	// r.Use(corsMiddleware)

	h := handlers.NewDeploymentHandler(srv)
	r.Route("/api", func(r chi.Router) {
		r.Get("/deployments", h.ListDeployments)
		r.Post("/deployments", h.CreateDeployment)
		r.Get("/deployments/{id}", h.GetDeployment)
		r.Delete("/deployments/{id}", h.DeleteDeployment)
		r.Post("/deployments/{id}/redeploy", h.RedeployDeployment)
		r.Post("/deployments/{id}/restart", h.RestartDeployment)
		r.Post("/deployments/{id}/rollback", h.RollbackDeployment)
		r.Get("/deployments/{id}/logs", h.StreamLogs) // SSE
	})

	srv.Router = r
}
