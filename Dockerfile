FROM golang:1.25.4 AS builder
WORKDIR /src

# Pre-cache modules for better layer reuse
COPY go.mod go.sum ./
RUN go mod download

# Copy source
COPY . .

# Build statically with size optimizations
ENV CGO_ENABLED=0
RUN go build -trimpath -ldflags "-s -w" -o /out/bot .

# Runtime image
FROM alpine:3.20

ENV BanDataDir=/ban/data \
    BanStratDir=/ban/strats

# Prepare directories
RUN mkdir -p /ban/strats_init /ban/strats /ban/data

# Copy bot binary
COPY --from=builder /out/bot /ban/bot

# Copy strategy sources and scripts for initialization
COPY --from=builder /src /ban/strats_init

# Ensure required executables are runnable
RUN chmod +x /ban/bot && chmod +x /ban/strats_init/scripts/run.sh

EXPOSE 8000 8001

WORKDIR /ban/strats

ENTRYPOINT ["/ban/strats_init/scripts/run.sh", "/ban/bot"]
