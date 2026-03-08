# ── Stage 1: build ────────────────────────────────────────────────────────────
FROM golang:1.22-alpine AS build
WORKDIR /src

COPY go.mod .
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /carter-webhook .

# ── Stage 2: runtime ───────────────────────────────────────────────────────────
FROM alpine:3.19
RUN apk add --no-cache bash curl

# Non-root user
RUN addgroup -S appgroup && adduser -S appuser -G appgroup

WORKDIR /app
COPY --from=build /carter-webhook .

USER appuser

ENV PORT=8080
EXPOSE 8080

ENTRYPOINT ["/app/carter-webhook"]