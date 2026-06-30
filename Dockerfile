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
# `shell:` steps, which require /bin/sh in the image. Runs as root so it can
# write its .lp output into the shared workspace alongside files created by
# the (typically root) eval step.
FROM alpine:3.20
COPY --from=build /out/testkube-ai-metrics-parser /usr/local/bin/testkube-ai-metrics-parser
ENTRYPOINT ["testkube-ai-metrics-parser"]
