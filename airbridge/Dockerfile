# Global ARG for Home Assistant base image
ARG BUILD_FROM=ghcr.io/hassio-addons/base:16.0.1

# Build cliraop from source
FROM debian:bookworm-slim AS cliraop-builder

RUN apt-get update && apt-get install -y --no-install-recommends \
    build-essential \
    cmake \
    git \
    libssl-dev \
    ca-certificates \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /src
RUN git clone https://github.com/philippe44/libraop.git . \
    && git submodule update --init --recursive

WORKDIR /src/build
RUN cmake .. && make

# Build airbridge
FROM golang:1.24-bookworm AS go-builder

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /airbridge ./cmd/airbridge

# Final image using Home Assistant base
ARG BUILD_FROM
FROM ${BUILD_FROM}

# Copy binaries from build stages
COPY --from=cliraop-builder /src/build/raop_play-linux-* /usr/local/bin/cliraop
COPY --from=go-builder /airbridge /usr/local/bin/airbridge

RUN chmod +x /usr/local/bin/cliraop /usr/local/bin/airbridge

WORKDIR /data

# Setup s6-overlay v3 service
RUN mkdir -p /etc/s6-overlay/s6-rc.d/airbridge/dependencies.d \
    && touch /etc/s6-overlay/s6-rc.d/airbridge/dependencies.d/base \
    && echo "longrun" > /etc/s6-overlay/s6-rc.d/airbridge/type \
    && touch /etc/s6-overlay/s6-rc.d/user/contents.d/airbridge

COPY airbridge/run.sh /etc/s6-overlay/s6-rc.d/airbridge/run
RUN chmod +x /etc/s6-overlay/s6-rc.d/airbridge/run
