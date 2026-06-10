# syntax=docker/dockerfile:1
# Reviewer image: the binaries the core component's review jobs run.

FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o /out/claude-review ./cmd/claude-review \
    && CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o /out/gemini-review ./cmd/gemini-review \
    && CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o /out/mr-gate ./cmd/mr-gate

FROM alpine:3.24
# ca-certificates is required for HTTPS to the GitLab, Anthropic, and Gemini APIs.
RUN apk add --no-cache ca-certificates
COPY --from=build /out/claude-review /usr/local/bin/claude-review
COPY --from=build /out/gemini-review /usr/local/bin/gemini-review
COPY --from=build /out/mr-gate /usr/local/bin/mr-gate
