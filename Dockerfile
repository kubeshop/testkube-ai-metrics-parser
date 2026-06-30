# syntax=docker/dockerfile:1
FROM --platform=$BUILDPLATFORM golang:1.23-alpine AS build
WORKDIR /src
COPY go.mod ./
RUN go mod download
COPY . .
ARG TARGETOS TARGETARCH VERSION=dev
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -trimpath -ldflags "-s -w -X main.version=${VERSION}" \
    -o /out/testkube-ai-metrics-parser ./cmd/testkube-ai-metrics-parser

# Alpine (not distroless) so the binary can be invoked from TestWorkflow
# `shell:` steps, which require /bin/sh in the image.
FROM alpine:3.20
RUN adduser -D -u 65532 nonroot
COPY --from=build /out/testkube-ai-metrics-parser /usr/local/bin/testkube-ai-metrics-parser
USER nonroot
ENTRYPOINT ["testkube-ai-metrics-parser"]
