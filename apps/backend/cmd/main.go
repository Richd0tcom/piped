package main

import (
	"fmt"
	"log"

	"github.com/joho/godotenv"
	"github.com/richd0tcom/piped/config"
	"github.com/richd0tcom/piped/core/routing"
	"github.com/richd0tcom/piped/core/server"
	"github.com/richd0tcom/piped/internal/filemanager"
	"github.com/richd0tcom/piped/internal/maestro"
	"github.com/richd0tcom/piped/internal/portal"
	"github.com/richd0tcom/piped/internal/proxy"
	"github.com/richd0tcom/piped/internal/store"
	"github.com/richd0tcom/piped/internal/vessel"
)

func main() {

	_ = godotenv.Load()

	cfg:= config.InitViper()


	fmt.Println("DB_PATH:", cfg.GetString(config.EnvDBPath))
	s, err := store.New(cfg.GetString(config.EnvDBPath))
	
	must(err, "store")

	v, err := vessel.New()
	must(err, "vessel")

	fm, err := filemanager.New(cfg.GetString(config.EnvBuildDir))
	must(err, "filemanager")

	p := portal.New(s)
	defer p.Close()

	px := proxy.New(cfg.GetString(config.EnvCaddyURL))

	m := maestro.New(s, p, v, px, fm)

	a, err := server.NewServer(cfg, s, p, m, cfg.GetString(config.EnvUploadDir))
	must(err, "server")

	routing.SetupRouter(a)

	server.RunServer(a)
}



func must(err error, label string) {
	if err != nil {
		log.Fatalf("init %s: %v", label, err)
	}
}