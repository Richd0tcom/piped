package server

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/richd0tcom/piped/config"
	"github.com/richd0tcom/piped/internal/maestro"
	"github.com/richd0tcom/piped/internal/portal"
	"github.com/richd0tcom/piped/internal/store"
	"github.com/spf13/viper"
)

const (
	idleTimeOut     = 60
	readTimeOut     = 60
	writeTimOut     = 60
	timeOutDuration = 60
)

type Server struct {


	Config   *viper.Viper
	Store   *store.Store
	Portal  *portal.Portal
	Maestro *maestro.Maestro
	Router *chi.Mux
}

func NewServer(cfg *viper.Viper, store *store.Store, portal *portal.Portal, mstro *maestro.Maestro, dir string) (*Server, error) {


	return &Server{
		Config:  cfg,
		Store:    store,
		// Live:     live,
		Portal: portal,
		Maestro: mstro,
	}, nil
}

func RunServer(srv *Server) {
	// Setup server
	srv.Config.GetString(config.Env)

	port := srv.Config.GetString(config.EnvPort) // e.g., "8080"

    addr := fmt.Sprintf("0.0.0.0:%s", port)
	httpServer := &http.Server{
		Addr:         addr,
		WriteTimeout: timeoutDuration(writeTimOut),
		ReadTimeout:  timeoutDuration(readTimeOut),
		IdleTimeout:  timeoutDuration(idleTimeOut),
		Handler:      srv.Router, 
	}

	// Run our server in a goroutine so that it doesn't block.
	go func() {
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatal("listen: ", err)
		}
	}()

	log.Println("Server is up and running on: ", addr)

	quit := make(chan os.Signal, 1)
	// Accept graceful shutdowns when quit via SIGINT (Ctrl+C)
	// kill (no param) default send syscall.SIGTERM
	// kill -2 is syscall.SIGINT
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	// Block until we receive our signal.
	<-quit

	log.Println("Shutting down server...")

	// Create a deadline to wait for.
	ctx, cancel := context.WithTimeout(context.Background(), timeoutDuration(timeOutDuration))
	defer cancel()

	// Shutdown the server.
	if err := httpServer.Shutdown(ctx); err != nil {
		log.Println("Server forced to shutdown: ", err)
	}
}

func timeoutDuration(second int) time.Duration {
	return time.Second * time.Duration(second)
}


