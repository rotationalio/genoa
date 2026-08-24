#!/bin/bash

GIT_REVISION=abc123 BUILD_DATE=$(date +%Y-%m-%d) \
    go build -o genoa \
    -ldflags="-X 'go.rtnl.ai/genoa.GitVersion=${GIT_REVISION}'" \
    -ldflags="-X 'go.rtnl.ai/genoa.BuildDate=${BUILD_DATE}'" \
    ./cmd/genoa