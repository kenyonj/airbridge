ARG BUILD_FROM
FROM ${BUILD_FROM}

# Install dependencies
RUN apk add --no-cache \
    ca-certificates \
    openssl \
    build-base \
    cmake \
    git \
    openssl-dev \
    go

# Build cliraop from source
WORKDIR /tmp/libraop
RUN git clone https://github.com/philippe44/libraop.git . \
    && git submodule update --init --recursive \
    && mkdir build && cd build \
    && cmake .. && make \
    && cp raop_play-linux-* /usr/local/bin/cliraop \
    && chmod +x /usr/local/bin/cliraop

# Build airbridge
WORKDIR /tmp/airbridge
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /usr/local/bin/airbridge ./cmd/airbridge

# Cleanup build dependencies
RUN apk del build-base cmake git go openssl-dev \
    && rm -rf /tmp/libraop /tmp/airbridge /root/go

WORKDIR /data

# Copy run script
COPY homeassistant/airbridge/run.sh /run.sh
RUN chmod +x /run.sh

CMD ["/run.sh"]
