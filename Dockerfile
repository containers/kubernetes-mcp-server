FROM --platform=$BUILDPLATFORM golang:latest AS builder

ARG TARGETOS=linux
ARG TARGETARCH=amd64

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY ./ ./

RUN make build-multiarch TARGETOS=${TARGETOS} TARGETARCH=${TARGETARCH}

FROM registry.access.redhat.com/ubi9/ubi-minimal:latest
LABEL io.modelcontextprotocol.server.name="io.github.containers/kubernetes-mcp-server"
COPY mcp-config-container-defaults.toml /etc/kubernetes-mcp-server/config.toml
WORKDIR /app
COPY --from=builder /app/kubernetes-mcp-server /app/kubernetes-mcp-server
USER 65532:65532
ENV MCP_CONFIG_PATH=/etc/kubernetes-mcp-server/config.toml
ENTRYPOINT ["/app/kubernetes-mcp-server"]
EXPOSE 8080
