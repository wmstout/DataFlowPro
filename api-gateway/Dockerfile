FROM golang:1.17 AS builder

WORKDIR /app

# Generate self-signed certificate for HTTPS
RUN openssl req -x509 -newkey rsa:2048 -keyout key.pem -out cert.pem -days 365 -nodes \
    -subj "/C=US/ST=State/L=City/O=DataFlow/CN=localhost"

COPY go.mod ./
COPY main.go ./

# -mod=mod fetches dependencies and generates go.sum on the fly without removing unused deps
RUN go build -mod=mod -o gateway .

FROM debian:bullseye-slim

RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates \
    curl \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app

COPY --from=builder /app/gateway /app/gateway
COPY --from=builder /app/cert.pem /app/cert.pem
COPY --from=builder /app/key.pem /app/key.pem

# Seed export directory for G5 path traversal
RUN mkdir -p /data/exports /data/reports /etc/dataflow \
    && echo "2.4.1-enterprise" > /etc/dataflow/version \
    && echo "id,name,rows\n1,dataset_1,100" > /data/exports/dataset_1.csv \
    && echo "DataFlow Summary Report" > /data/reports/report.txt

EXPOSE 8443

HEALTHCHECK --interval=30s --timeout=5s --retries=3 \
    CMD curl -k -f https://localhost:8443/api/v1/health || exit 1

CMD ["/app/gateway"]
