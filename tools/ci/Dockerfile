# syntax=docker/dockerfile:1
# Multi-stage Dockerfile constructing the reviewer container image for CI pipeline jobs.

FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o /out/claude-review ./cmd/claude-review \
    && CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o /out/gemini-review ./cmd/gemini-review \
    && CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o /out/mr-gate ./cmd/mr-gate \
    && CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o /out/mr-labeler ./cmd/mr-labeler \
    && CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o /out/auto-tag ./cmd/auto-tag \
    && CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o /out/fmt-commit-pusher ./cmd/fmt-commit-pusher

FROM alpine:3.24
# ca-certificates provides TLS roots for API access. nodejs and npm run commitlint.
# git is required because commitlint executes git subcommands (e.g. git merge-base).
RUN apk add --no-cache ca-certificates nodejs npm git \
    && npm install -g --no-save @commitlint/cli@21.2.1 @commitlint/config-conventional@21.2.0
ENV NODE_PATH=/usr/local/lib/node_modules
COPY --from=build /out/claude-review /usr/local/bin/claude-review
COPY --from=build /out/gemini-review /usr/local/bin/gemini-review
COPY --from=build /out/mr-gate /usr/local/bin/mr-gate
COPY --from=build /out/mr-labeler /usr/local/bin/mr-labeler
COPY --from=build /out/auto-tag /usr/local/bin/auto-tag
COPY --from=build /out/fmt-commit-pusher /usr/local/bin/fmt-commit-pusher
COPY commitlint.config.cjs /etc/commitlint/commitlint.config.cjs
