package main

import (
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"strings"
)

// DeployRequest is the JSON body sent by GitHub Actions
type DeployRequest struct {
	Project string `json:"project"` // e.g. "bin-collection"
	Image   string `json:"image"`   // e.g. "ghcr.io/adammanderson/bin-collection-service:latest"
	Port    string `json:"port"`    // e.g. "8001"
}

var (
	// Allowlist: only lowercase alphanumeric and hyphens
	safeProject = regexp.MustCompile(`^[a-z0-9-]+$`)
	// Allowlist: ghcr.io images only
	safeImage = regexp.MustCompile(`^ghcr\.io/[a-zA-Z0-9._/-]+:[a-zA-Z0-9._-]+$`)
	// Allowlist: numeric port in valid range
	safePort = regexp.MustCompile(`^[0-9]{4,5}$`)
)

func main() {
	secret := os.Getenv("WEBHOOK_SECRET")
	if secret == "" {
		slog.Error("WEBHOOK_SECRET is not set")
		os.Exit(1)
	}

	deployScript := os.Getenv("DEPLOY_SCRIPT")
	if deployScript == "" {
		deployScript = "/srv/projects/deploy.sh"
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	mux := http.NewServeMux()

	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprintln(w, "ok")
	})

	mux.HandleFunc("POST /deploy", func(w http.ResponseWriter, r *http.Request) {
		log := slog.With("remote", r.RemoteAddr)

		// Authorise
		auth := r.Header.Get("Authorization")
		token, _ := strings.CutPrefix(auth, "Bearer ")
		if subtle.ConstantTimeCompare([]byte(token), []byte(secret)) != 1 {
			log.Warn("unauthorised deploy attempt")
			http.Error(w, "unauthorised", http.StatusUnauthorized)
			return
		}

		// Parse body
		var req DeployRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid JSON", http.StatusBadRequest)
			return
		}

		// Validate inputs — never pass unsanitised values to exec
		if !safeProject.MatchString(req.Project) {
			http.Error(w, "invalid project name", http.StatusBadRequest)
			return
		}
		if !safeImage.MatchString(req.Image) {
			http.Error(w, "invalid image", http.StatusBadRequest)
			return
		}
		if !safePort.MatchString(req.Port) {
			http.Error(w, "invalid port", http.StatusBadRequest)
			return
		}

		log.Info("deploying", "project", req.Project, "image", req.Image, "port", req.Port)

		// Run deploy.sh
		cmd := exec.Command("bash", deployScript, req.Project, req.Image, req.Port)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr

		if err := cmd.Run(); err != nil {
			log.Error("deploy failed", "err", err)
			http.Error(w, "deploy failed", http.StatusInternalServerError)
			return
		}

		log.Info("deploy complete", "project", req.Project)
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, "deployed %s\n", req.Project)
	})

	slog.Info("carter-webhook listening", "port", port)
	if err := http.ListenAndServe(":"+port, mux); err != nil {
		slog.Error("server error", "err", err)
		os.Exit(1)
	}
}