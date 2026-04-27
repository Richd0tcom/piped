package config

import (
	"github.com/spf13/viper"
)

const (
	Env          = "ENV"
	EnvPort      = "PORT"
	EnvDBPath    = "DB_PATH"
	EnvUploadDir = "UPLOAD_DIR"
	EnvBuildDir  = "BUILD_DIR"
	EnvCaddyURL  = "CADDY_ADMIN_URL"
)


func InitViper() *viper.Viper {
    v := viper.GetViper()
    v.AutomaticEnv()
    
    // Set all defaults here
    v.SetDefault(Env, "development")
    v.SetDefault(EnvPort, "8080")

    if v.GetString(Env) == "development" {
        
        v.SetDefault(EnvDBPath, "./piped.db")
        v.SetDefault(EnvUploadDir, "./uploads")
        v.SetDefault(EnvBuildDir, "./builds")
        v.SetDefault(EnvCaddyURL, "http://localhost:2019")
    } else {
        // Docker defaults
        v.SetDefault(EnvDBPath, "/data/piped.db")
        v.SetDefault(EnvUploadDir, "/tmp/piped/uploads")
        v.SetDefault(EnvBuildDir, "/tmp/piped/builds")
        v.SetDefault(EnvCaddyURL, "http://caddy:2019")
    }
    
    return v
}
 
// Simple getter
func getEnv(key string) string {
    return viper.GetString(key)
}
