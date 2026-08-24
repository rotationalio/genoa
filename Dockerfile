ARG BUILDER_IMAGE=golang:1.26-bookworm
ARG FINAL_IMAGE=debian:bookworm-slim

# Build stage
FROM --platform=${BUILDPLATFORM} ${BUILDER_IMAGE} AS builder

# Use modules for dependencies
WORKDIR $GOPATH/src/go.rtnl.ai/genoa

COPY go.mod .
COPY go.sum .

# Build args
ARG GIT_REVISION=""
ARG BUILD_DATE=""

ENV CGO_ENABLED=0
ENV GO111MODULE=on
RUN go mod download && go mod verify

# Copy source code
COPY . .

# Build the Genoa binary
RUN GOOS=${TARGETOS} GOARCH=${TARGETARCH} BUILD_DATE=$(date +%Y-%m-%d) \
    go build -o /go/bin/genoa \
    -ldflags="-X 'go.rtnl.ai/genoa.GitVersion=${GIT_REVISION}' -X 'go.rtnl.ai/genoa.BuildDate=${BUILD_DATE}'" \
    ./cmd/genoa

# Final stage
FROM ${FINAL_IMAGE} AS final

LABEL maintainer="Rotational Labs, Inc. <support@rotational.dev>"
LABEL description="A small kubernetes job that runs in front of a Rotational deployment to ensure the environment is setup."

# Copy the Genoa binary from the builder stage
COPY --from=builder /go/bin/genoa /usr/local/bin/genoa

CMD [ "/usr/local/bin/genoa" ]