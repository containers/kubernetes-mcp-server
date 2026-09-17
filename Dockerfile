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
WORKDIR /app
COPY --from=builder /app/kubernetes-mcp-server /app/kubernetes-mcp-server
RUN mkdir -p /etc/kubernetes-mcp-server && printf 'port = "8080"\n' > /etc/kubernetes-mcp-server/config.toml
USER 65532:65532
ENV MCP_CONFIG_PATH=/etc/kubernetes-mcp-server/config.toml
ENTRYPOINT ["/app/kubernetes-mcp-server"]
EXPOSE 8080
