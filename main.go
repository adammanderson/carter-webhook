package main

import (
	"crypto/subtle"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"strings"
)

var (
	safeProject = regexp.MustCompile(`^[a-z0-9-]+$`)
	safeImage   = regexp.MustCompile(`^ghcr\.io/[a-zA-Z0-9._/-]+:[a-zA-Z0-9._-]+$`)
)

func run(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func main() {
	secret := os.Getenv("WEBHOOK_SECRET")
	if secret == "" {
		slog.Error("WEBHOOK_SECRET is not set")
		os.Exit(1)
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

	// POST /redeploy?project=bin-collection&image=ghcr.io/...
	mux.HandleFunc("POST /redeploy", func(w http.ResponseWriter, r *http.Request) {
		log := slog.With("remote", r.RemoteAddr)

		// Authorise
		token, _ := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
		if subtle.ConstantTimeCompare([]byte(token), []byte(secret)) != 1 {
			log.Warn("unauthorised")
			http.Error(w, "unauthorised", http.StatusUnauthorized)
			return
		}

		project := r.URL.Query().Get("project")
		image := r.URL.Query().Get("image")

		if !safeProject.MatchString(project) {
			http.Error(w, "invalid project", http.StatusBadRequest)
			return
		}
		if !safeImage.MatchString(image) {
			http.Error(w, "invalid image", http.StatusBadRequest)
			return
		}

		log.Info("redeploying", "project", project, "image", image)

		// Pull new image
		if err := run("docker", "pull", image); err != nil {
			log.Error("pull failed", "err", err)
			http.Error(w, "pull failed", http.StatusInternalServerError)
			return
		}

		// Update the container image tag then restart via compose
		composeFile := fmt.Sprintf("/srv/projects/%s/docker-compose.yml", project)
		if _, err := os.Stat(composeFile); err == nil {
			// Compose file exists — use docker compose up
			if err := run("docker", "compose", "-f", composeFile, "up", "-d", "--no-build"); err != nil {
				log.Error("compose up failed", "err", err)
				http.Error(w, "compose up failed", http.StatusInternalServerError)
				return
			}
		} else {
			// No compose file — just stop/rm/run won't work without original args
			// Force recreate by stopping and letting Portainer recreate
			run("docker", "stop", project)
			run("docker", "rm", project)
			log.Warn("no compose file found, container removed — Portainer will need to recreate", "project", project)
			http.Error(w, "no compose file for "+project, http.StatusInternalServerError)
			return
		}

		log.Info("redeploy complete", "project", project)
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, "redeployed %s\n", project)
	})

	slog.Info("carter-webhook listening", "port", port)
	if err := http.ListenAndServe(":"+port, mux); err != nil {
		slog.Error("server error", "err", err)
		os.Exit(1)
	}
}