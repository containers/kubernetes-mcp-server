FROM --platform=$BUILDPLATFORM golang:latest AS builder

ARG TARGETOS=linux
ARG TARGETARCH=amd64

WORKDIR /app
COPY ./ ./

RUN make build-multiarch TARGETOS=${TARGETOS} TARGETARCH=${TARGETARCH}

FROM registry.access.redhat.com/ubi9/ubi-minimal:latest
LABEL io.modelcontextprotocol.server.name="io.github.containers/kubernetes-mcp-server"
WORKDIR /app
COPY --from=builder /app/kubernetes-mcp-server /app/kubernetes-mcp-server
USER 65532:65532
ENTRYPOINT ["/app/kubernetes-mcp-server"]
CMD ["--port", "8080"]
EXPOSE 8080
