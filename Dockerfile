# Stage 1: Build Next.js frontend
FROM node:22-alpine AS frontend-builder

WORKDIR /app/dashboard

# Copy package files
COPY dashboard/package*.json ./

# Install dependencies
RUN npm ci

# Copy source files
COPY dashboard/ ./

# Build Next.js (standalone output)
RUN npm run build

# Stage 2: Python runtime with both services
FROM python:3.11-slim

WORKDIR /app

# Install system dependencies
RUN apt-get update && apt-get install -y --no-install-recommends \
    supervisor \
    nodejs \
    && rm -rf /var/lib/apt/lists/*

# Install uv
COPY --from=ghcr.io/astral-sh/uv:latest /uv /usr/local/bin/uv

# Copy Python project files
COPY pyproject.toml ./
COPY memcp/ ./memcp/

# Install Python dependencies with webui extras
RUN uv pip install --system -e ".[webui]"

# Copy Next.js standalone build from frontend stage
COPY --from=frontend-builder /app/dashboard/.next/standalone ./dashboard/.next/standalone
COPY --from=frontend-builder /app/dashboard/.next/static ./dashboard/.next/standalone/.next/static

# Copy supervisord config
COPY supervisord.conf /etc/supervisor/conf.d/supervisord.conf

# Expose ports
EXPOSE 3000 8080

# Environment variables with defaults
ENV SURREALDB_URL=ws://localhost:8000/rpc \
    SURREALDB_NAMESPACE=knowledge \
    SURREALDB_DATABASE=graph \
    SURREALDB_USER=root \
    SURREALDB_PASS=root \
    SURREALDB_AUTH_LEVEL=root \
    HOSTNAME=0.0.0.0 \
    PORT=3000

# Run supervisord
CMD ["/usr/bin/supervisord", "-c", "/etc/supervisor/conf.d/supervisord.conf"]
