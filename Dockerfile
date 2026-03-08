# ── Stage 1: build ────────────────────────────────────────────────────────────
FROM golang:1.22-alpine AS build
WORKDIR /src

COPY go.mod .
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /carter-webhook .

# ── Stage 2: runtime ───────────────────────────────────────────────────────────
# distroless = no shell, no package manager, minimal attack surface
FROM gcr.io/distroless/static:nonroot
COPY --from=build /carter-webhook /carter-webhook

ENV PORT=8080

EXPOSE 8080

ENTRYPOINT ["/carter-webhook"]