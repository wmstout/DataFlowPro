FROM python:3.9-slim

LABEL maintainer="platform-team@dataflow.io"
LABEL version="2.4.1"

# Install system dependencies
RUN apt-get update && apt-get install -y \
    curl \
    wget \
    netcat-openbsd \
    vim \
    gcc \
    make \
    libc6-dev \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app

# Copy application code
COPY data-service/ /app/
COPY .env /app/.env

# Build the C processing engine
COPY processing-core/ /opt/dataflow/engine-src/
RUN make -C /opt/dataflow/engine-src \
    && cp /opt/dataflow/engine-src/dataflow-engine /usr/local/bin/dataflow-engine

# Runtime data directories and helper scripts
COPY seed/data/ /data/
COPY seed/opt/ /opt/dataflow/
COPY processing-core/data/sample.csv /data/datasets/sample.csv
RUN mkdir -p /data/uploads

# Install Python dependencies
RUN pip install --no-cache-dir -r requirements.txt

# Expose service port and debug port
EXPOSE 5000
EXPOSE 5001

# Health check
HEALTHCHECK --interval=30s --timeout=5s --retries=3 \
    CMD curl -f http://localhost:5000/api/v1/health || exit 1

# Run as root (required for bind mount access)
USER root

# Start the application
ENV FLASK_ENV=development
ENV FLASK_DEBUG=1

CMD ["python", "app.py"]
