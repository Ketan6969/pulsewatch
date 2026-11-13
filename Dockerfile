# syntax=docker/dockerfile:1

# ---- Dashboard build ----
FROM node:22-alpine AS web
WORKDIR /web
COPY web/package.json web/package-lock.json ./
RUN npm ci --no-audit --no-fund
COPY web/ ./
RUN npm run build

# ---- Go build ----
FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
ARG VERSION=dev
RUN CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.version=${VERSION}" -o /out/pulsewatch ./cmd/pulsewatch \
 && CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.version=${VERSION}" -o /out/pulsewatch-agent ./cmd/pulsewatch-agent

# ---- Server image ----
FROM gcr.io/distroless/static-debian12:nonroot AS server
COPY --from=build /out/pulsewatch /usr/local/bin/pulsewatch
COPY --from=web /web/dist /srv/web
ENV PULSEWATCH_HTTP_ADDR=:8080 \
    PULSEWATCH_WEB_DIR=/srv/web
EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/usr/local/bin/pulsewatch"]

# ---- Agent image ----
# Runs as root by default so it can read /proc of other users' processes on the host.
FROM gcr.io/distroless/static-debian12 AS agent
COPY --from=build /out/pulsewatch-agent /usr/local/bin/pulsewatch-agent
ENTRYPOINT ["/usr/local/bin/pulsewatch-agent"]
