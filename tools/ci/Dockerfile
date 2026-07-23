# syntax=docker/dockerfile:1
# Multi-stage Dockerfile constructing the reviewer container image for CI pipeline jobs.

FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o /out/claude-review ./cmd/claude-review \
    && CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o /out/gemini-review ./cmd/gemini-review \
    && CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o /out/mr-gate ./cmd/mr-gate

FROM alpine:3.24
# ca-certificates provides root TLS certificates required for secure HTTPS communication with
# GitLab, Anthropic, and Gemini APIs. nodejs and npm are included exclusively for commitlint
# CLI execution. Exact package version pinning guarantees reproducible commit rule enforcement.
RUN apk add --no-cache ca-certificates nodejs npm \
    && npm install -g --no-save @commitlint/cli@21.2.1 @commitlint/config-conventional@21.2.0
COPY --from=build /out/claude-review /usr/local/bin/claude-review
COPY --from=build /out/gemini-review /usr/local/bin/gemini-review
COPY --from=build /out/mr-gate /usr/local/bin/mr-gate
COPY commitlint.config.cjs /etc/commitlint/commitlint.config.cjs
