# Copyright (c) 2026 McSparrow. All rights reserved.
# McHarbor is licensed under the McHarbor License. See LICENSE for details.

# Stage 1: Go backend
FROM golang:1.26-alpine AS backend
RUN apk add --no-cache git
WORKDIR /app
COPY src/backend/go.mod ./
COPY src/backend/go.sum* ./
RUN go mod download
COPY src/backend/ .
RUN go mod tidy && CGO_ENABLED=0 go build -ldflags="-s -w" -o /mcharbor ./main.go

# Stage 2: React frontend
FROM node:22-alpine AS frontend
WORKDIR /app
COPY src/frontend/package.json src/frontend/package-lock.json* ./
RUN npm install
COPY src/frontend/ .
RUN npm run build

# Stage 3: Production
FROM alpine:3.20
RUN apk add --no-cache tini docker-cli docker-cli-compose sqlite curl bash ca-certificates tzdata

# Install Trivy vulnerability scanner
RUN curl -sfL https://raw.githubusercontent.com/aquasecurity/trivy/main/contrib/install.sh | sh -s -- -b /usr/local/bin

# Install Grype vulnerability scanner
RUN curl -sSfL https://raw.githubusercontent.com/anchore/grype/main/install.sh | sh -s -- -b /usr/local/bin

WORKDIR /app

COPY --from=backend /mcharbor ./mcharbor
COPY --from=frontend /app/dist ./static
COPY blueprints/ ./blueprints/
COPY docker/docker-entrypoint.sh /docker-entrypoint.sh

RUN chmod +x /docker-entrypoint.sh && mkdir -p /app/data

ENV PORT=5474
EXPOSE 5474

HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
    CMD curl -f -k https://localhost:5474/api/health || curl -f http://localhost:5474/api/health || exit 1

ENTRYPOINT ["tini", "--", "/docker-entrypoint.sh"]
CMD ["./mcharbor"]
