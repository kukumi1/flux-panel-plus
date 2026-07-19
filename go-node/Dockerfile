# syntax=docker/dockerfile:1.7

FROM --platform=$BUILDPLATFORM golang:1.25-alpine AS builder
ARG TARGETOS
ARG TARGETARCH
ARG VERSION=dev
WORKDIR /src
COPY go.mod go.sum ./
COPY x/go.mod x/go.sum ./x/
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -trimpath -buildvcs=false \
    -ldflags="-s -w -X main.nodeVersion=${VERSION}" \
    -o /out/node-svc .

FROM alpine:3.22

ARG TARGETARCH
ARG XRAY_VERSION=25.1.30

RUN apk add --no-cache ca-certificates iproute2 wget unzip

# Install secondary binary (optional, controlled at runtime)
RUN XRAY_ARCH=$(case ${TARGETARCH} in amd64) echo "64";; arm64) echo "arm64-v8a";; *) echo "64";; esac) && \
    wget -q "https://github.com/XTLS/Xray-core/releases/download/v${XRAY_VERSION}/Xray-linux-${XRAY_ARCH}.zip" -O /tmp/svc.zip && \
    unzip -q /tmp/svc.zip -d /tmp/svc && \
    mv /tmp/svc/xray /usr/local/bin/svc-runtime && \
    chmod +x /usr/local/bin/svc-runtime && \
    rm -rf /tmp/svc /tmp/svc.zip || true

WORKDIR /etc/node

COPY --from=builder /out/node-svc /usr/local/bin/node-svc
RUN chmod +x /usr/local/bin/node-svc

COPY docker-entrypoint.sh /docker-entrypoint.sh
RUN chmod +x /docker-entrypoint.sh

ENTRYPOINT ["/docker-entrypoint.sh"]
