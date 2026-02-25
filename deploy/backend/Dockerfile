# context: .  (repo root)
FROM golang:1.22-alpine AS builder

RUN apk add --no-cache gcc musl-dev

WORKDIR /src
COPY backend/go.mod backend/go.sum ./
RUN go mod download

COPY backend/ .
RUN CGO_ENABLED=1 GOOS=linux \
    go build -ldflags="-s -w" -o /app/server ./

# kubectl binary indir
FROM alpine:3.20 AS kubectl-dl
RUN apk add --no-cache curl
RUN KUBECTL_VERSION=$(curl -sL https://dl.k8s.io/release/stable.txt) && \
    curl -sLo /kubectl "https://dl.k8s.io/release/${KUBECTL_VERSION}/bin/linux/amd64/kubectl" && \
    chmod +x /kubectl

FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata wget
WORKDIR /app
COPY --from=builder /app/server .
COPY --from=kubectl-dl /kubectl /usr/local/bin/kubectl
RUN mkdir -p /data /root/.kube
EXPOSE 8080
ENV APP_ENV=production PORT=8080 DB_PATH=/data/platr.db
CMD ["/app/server"]