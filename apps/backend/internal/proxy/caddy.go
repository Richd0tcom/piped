package proxy

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
)

type Proxy struct {
	adminURL   string
	serverName string // resolved once on first use
}

func New(adminURL string) *Proxy {
	return &Proxy{adminURL: adminURL}
}

// caddyRoute matches Caddy's route object schema.
// The @id field makes routes addressable for update/delete.
type caddyRoute struct {
	ID       string        `json:"@id"`
	Match    []caddyMatch  `json:"match"`
	Handle   []caddyHandle `json:"handle"`
	Terminal bool          `json:"terminal"`
}

type caddyMatch struct {
	Path []string `json:"path"`
}

type caddyHandle struct {
	Handler   string          `json:"handler"`
	Upstreams []caddyUpstream `json:"upstreams,omitempty"`
}

type caddyUpstream struct {
	Dial string `json:"dial"`
}

// creates a new reverse proxy route for a deployment.
func (p *Proxy) AddRoute(deploymentID string, port int) error {
	route := p.buildRoute(deploymentID, port)
	body, err := json.Marshal(route)
	if err != nil {
		return err
	}
	server, err := p.resolveServer()
	if err != nil {
		return err
	}
	url := fmt.Sprintf("%s/config/apps/http/servers/%s/routes", p.adminURL, server)
	return p.do(http.MethodPost, url, body)
}

// SwapRoute updates the upstream for an existing route in place (blue-green cutover).
// Uses the @id field to address the route directly — no duplicate routes.
func (p *Proxy) SwapRoute(deploymentID string, newPort int) error {
	route := p.buildRoute(deploymentID, newPort)
	body, err := json.Marshal(route)
	if err != nil {
		return err
	}
	// PUT to the route's @id address patches it in place atomically
	url := fmt.Sprintf("%s/id/%s", p.adminURL, routeID(deploymentID))
	return p.do(http.MethodPut, url, body)
}

// RemoveRoute deletes a deployment's route using its @id.
func (p *Proxy) RemoveRoute(deploymentID string) error {
	url := fmt.Sprintf("%s/id/%s", p.adminURL, routeID(deploymentID))
	return p.do(http.MethodDelete, url, nil)
}

// resolveServer discovers the HTTP server name from Caddy's live config.
// Caddy names servers based on listen addresses — we can't assume "srv0".
func (p *Proxy) resolveServer() (string, error) {
	if p.serverName != "" {
		return p.serverName, nil
	}
	resp, err := http.Get(fmt.Sprintf("%s/config/apps/http/servers", p.adminURL))
	if err != nil {
		return "", fmt.Errorf("caddy resolve server: %w", err)
	}
	defer resp.Body.Close()

	var servers map[string]json.RawMessage
	if err := json.NewDecoder(resp.Body).Decode(&servers); err != nil {
		return "", fmt.Errorf("caddy decode servers: %w", err)
	}
	for name := range servers {
		p.serverName = name
		return name, nil
	}
	return "", fmt.Errorf("caddy: no http servers found in config")
}

func (p *Proxy) buildRoute(deploymentID string, port int) caddyRoute {
	return caddyRoute{
		ID:    routeID(deploymentID),
		Match: []caddyMatch{{Path: []string{fmt.Sprintf("/deploy/%s/*", deploymentID)}}},
		Handle: []caddyHandle{{
			Handler:   "reverse_proxy",
			Upstreams: []caddyUpstream{{Dial: fmt.Sprintf("127.0.0.1:%d", port)}},
		}},
		Terminal: true,
	}
}

// routeID produces a stable Caddy @id for a deployment.
func routeID(deploymentID string) string {
	return "piped-" + deploymentID
}

func (p *Proxy) do(method, url string, body []byte) error {
	var req *http.Request
	var err error
	if body != nil {
		req, err = http.NewRequest(method, url, bytes.NewReader(body))
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/json")
	} else {
		req, err = http.NewRequest(method, url, nil)
		if err != nil {
			return err
		}
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("caddy %s %s: %w", method, url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("caddy %s %s: status %d", method, url, resp.StatusCode)
	}
	return nil
}