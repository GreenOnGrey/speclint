# The image of speclint (FTR.HMR.SPEC-0002 R1, arch A2): the binary and git, which the mode service and -code
# use for git grep. The release workflow builds it for linux/amd64 and linux/arm64.
FROM --platform=$BUILDPLATFORM golang:1.27-alpine AS build
WORKDIR /src
COPY . .
ARG TARGETOS TARGETARCH VERSION=dev
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -ldflags "-s -w -X main.version=${VERSION}" -o /out/speclint ./cmd/speclint

FROM alpine:3.23
ARG VERSION=dev
RUN apk add --no-cache git \
 # the workspace of CI belongs to another user than the container
 && git config --system --add safe.directory '*'
COPY --from=build /out/speclint /usr/local/bin/speclint
LABEL org.opencontainers.image.title="speclint" \
      org.opencontainers.image.version="${VERSION}" \
      org.opencontainers.image.source="https://github.com/GreenOnGrey/speclint" \
      org.opencontainers.image.licenses="Apache-2.0"
ENTRYPOINT ["speclint"]
