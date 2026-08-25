FROM node:24-bookworm-slim AS frontend
WORKDIR /src
COPY frontend/package.json frontend/package-lock.json ./
RUN npm ci
COPY frontend/ ./
RUN npm run build

FROM golang:1.25-bookworm AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . ./
COPY --from=frontend /src/dist frontend/dist
RUN CGO_ENABLED=0 go build -tags headless -trimpath -buildvcs=false -ldflags="-s -w -buildid=" -o /axi .

FROM debian:bookworm-slim
ARG TARGETARCH
ARG AX_VERSION=v0.4.4
ARG AXIS_VERSION=v0.2.0
ARG FSX_VERSION=v0.1.0
ARG BASHX_VERSION=v0.2.1
ARG SKILLX_VERSION=v0.1.0
ARG ATTACHX_VERSION=v0.1.0
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates curl tini && rm -rf /var/lib/apt/lists/*
RUN set -eu; \
    case "$TARGETARCH" in amd64) arch=x86_64 ;; arm64) arch=aarch64 ;; *) exit 1 ;; esac; \
    for spec in "ax:$AX_VERSION" "axis:$AXIS_VERSION" "fsx:$FSX_VERSION" "bashx:$BASHX_VERSION" "skillx:$SKILLX_VERSION" "attachx:$ATTACHX_VERSION"; do \
        package=${spec%%:*}; \
        version=${spec#*:}; \
        artifact="$package-linux-$arch"; \
        base="https://github.com/3-lines-studio/$package/releases/download/$version"; \
        curl -fsSL "$base/$artifact" -o "/usr/local/bin/$package"; \
        curl -fsSL "$base/$artifact.sha256" -o "/tmp/$package.sha256"; \
        want=$(awk '{print $1}' "/tmp/$package.sha256"); \
        got=$(sha256sum "/usr/local/bin/$package" | awk '{print $1}'); \
        test "$want" = "$got"; \
        chmod 0755 "/usr/local/bin/$package"; \
    done; \
    rm -f /tmp/*.sha256
COPY --from=build /axi /usr/local/bin/axi
RUN useradd --create-home --uid 10001 axi && mkdir -p /data/projects && chown -R axi:axi /data
USER axi
WORKDIR /data/projects
ENV HOME=/home/axi
ENV AXI_HOME=/data
ENV XDG_CONFIG_HOME=/data/config
ENV AXI_WEB_ADDRESS=0.0.0.0:8080
EXPOSE 8080
VOLUME ["/data"]
HEALTHCHECK --interval=30s --timeout=3s --start-period=10s --retries=3 CMD curl -fsS http://127.0.0.1:8080/health -o /dev/null || exit 1
ENTRYPOINT ["/usr/bin/tini", "--"]
CMD ["axi", "web"]
