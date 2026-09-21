# syntax=docker/dockerfile:1
# Multi-stage build for CI reviewer binaries and commitlint execution environment.

FROM golang:1.26-alpine AS build
ARG CI_JOB_TOKEN
RUN apk add --no-cache git
ENV GOPRIVATE=gitlab.com/csning1998-lab/*
RUN if [ -n "$CI_JOB_TOKEN" ]; then \
      git config --global url."https://gitlab-ci-token:${CI_JOB_TOKEN}@gitlab.com/".insteadOf "https://gitlab.com/"; \
    fi
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o /out/review ./cmd/review \
    && CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o /out/mr-gate ./cmd/mr-gate \
    && CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o /out/mr-labeler ./cmd/mr-labeler \
    && CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o /out/auto-tag ./cmd/auto-tag \
    && CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o /out/verify-tag-token ./cmd/verify-tag-token \
    && CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o /out/fmt-commit-pusher ./cmd/fmt-commit-pusher

FROM alpine:3.24
# ca-certificates serves TLS validation, Node.js and npm serve commitlint, and Git serves the
# commitlint tree traversal subcommands (git merge-base). The bundled native tsc is removed
# because its prebuilt Go stdlib reports HIGH CVEs (CVE-2026-33818 and seven siblings).
RUN apk upgrade --no-cache \
    && apk add --no-cache ca-certificates nodejs npm git \
    && npm install -g --no-save @commitlint/cli@21.2.1 @commitlint/config-conventional@21.2.0 \
    && test -d /usr/local/lib/node_modules/@commitlint/cli/node_modules/@typescript \
    && rm -rf /usr/local/lib/node_modules/@commitlint/cli/node_modules/@typescript

ENV NODE_PATH=/usr/local/lib/node_modules
COPY --from=build /out/review /usr/local/bin/review
COPY --from=build /out/mr-gate /usr/local/bin/mr-gate
COPY --from=build /out/mr-labeler /usr/local/bin/mr-labeler
COPY --from=build /out/auto-tag /usr/local/bin/auto-tag
COPY --from=build /out/verify-tag-token /usr/local/bin/verify-tag-token
COPY --from=build /out/fmt-commit-pusher /usr/local/bin/fmt-commit-pusher
COPY commitlint.config.cjs /etc/commitlint/commitlint.config.cjs
