# Stage 1: Build Go binary
FROM golang:1.25-alpine AS go-builder
WORKDIR /build
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ .
RUN CGO_ENABLED=0 go build -o /app/server ./cmd/api

# Stage 2: Runtime with Python + Go binary
FROM python:3.13-slim AS runtime

# Playwright system dependencies (Chromium only)
RUN apt-get update && apt-get install -y --no-install-recommends \
    libglib2.0-0 libnss3 libnspr4 libdbus-1-3 libatk1.0-0 \
    libatk-bridge2.0-0 libcups2 libdrm2 libxcb1 libxkbcommon0 \
    libatspi2.0-0 libx11-6 libxcomposite1 libxdamage1 libxext6 \
    libxfixes3 libxrandr2 libgbm1 libpango-1.0-0 libcairo2 \
    libasound2 xvfb xauth && \
    rm -rf /var/lib/apt/lists/*

WORKDIR /app

# Install Python worker
COPY worker/ ./worker/
RUN pip install --no-cache-dir -e ./worker && \
    playwright install chromium

# Copy Go binary
COPY --from=go-builder /app/server ./server

# Create runtime directories
RUN mkdir -p /data /app/backend/uploads /app/backend/data-cache /app/worker/data

ENV DATABASE_URL=/data/portfolio.db
ENV UPLOAD_DIR=/app/backend/uploads
ENV DATA_CACHE_DIR=/app/backend/data-cache
ENV WORKER_DIR=/app/worker
ENV WORKER_PYTHON=python3
ENV HEADLESS=true

EXPOSE 8000
CMD ["./server"]
