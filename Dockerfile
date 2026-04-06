FROM golang:1.24-alpine AS builder

ENV GOTOOLCHAIN=auto
WORKDIR /build

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /winet2mqtt ./cmd/winet2mqtt

FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=builder /winet2mqtt /winet2mqtt

EXPOSE 8081

HEALTHCHECK --interval=10s --timeout=5s --start-period=30s --retries=3 \
    CMD ["/winet2mqtt", "health"]

ENTRYPOINT ["/winet2mqtt", "server"]
