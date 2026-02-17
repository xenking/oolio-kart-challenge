FROM golang:1.25-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o api-server ./cmd/api-server
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o coupon-ingest ./cmd/coupon-ingest
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o seed-db ./cmd/seed-db

FROM alpine:3.21
RUN apk --no-cache add ca-certificates && \
    addgroup -S appgroup && adduser -S appuser -G appgroup
COPY --from=builder /app/api-server /app/
COPY --from=builder /app/coupon-ingest /app/
COPY --from=builder /app/seed-db /app/
COPY --from=builder /app/config.yaml /app/
COPY --from=builder /app/db/seed/ /app/db/seed/
WORKDIR /app
USER appuser
EXPOSE 8080
ENTRYPOINT ["/app/api-server"]
