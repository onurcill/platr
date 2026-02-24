# context: grpc-inspector/
FROM golang:1.22-alpine AS builder

RUN apk add --no-cache gcc musl-dev

WORKDIR /src
COPY backend/go.mod backend/go.sum ./
RUN go mod download

COPY backend/ .
RUN CGO_ENABLED=1 GOOS=linux \
    go build -ldflags="-s -w" -o /app/server ./

FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata wget
WORKDIR /app
COPY --from=builder /app/server .
RUN mkdir -p /data
EXPOSE 8080
ENV APP_ENV=production PORT=8080 DB_PATH=/data/platr.db
CMD ["/app/server"]
