# syntax=docker/dockerfile:1
FROM registry.access.redhat.com/ubi9/go-toolset:1.22 AS build
WORKDIR /opt/app-root/src
COPY --chown=1001:0 go.mod go.sum ./
COPY --chown=1001:0 cmd ./cmd
RUN go mod download && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -trimpath -ldflags="-s -w" -o /opt/app-root/src/webhook ./cmd/webhook

FROM registry.access.redhat.com/ubi9/ubi-micro:latest
COPY --from=build /opt/app-root/src/webhook /usr/local/bin/webhook
USER 1001
EXPOSE 8443
ENTRYPOINT ["/usr/local/bin/webhook"]
