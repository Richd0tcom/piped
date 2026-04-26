# Multi-stage build for production
# Stage 1: Build
FROM oven/bun:1 AS builder

WORKDIR /app

# Copy package files
COPY package.json bun.lockb ./

# Install dependencies (production only for smaller image)
RUN bun install --frozen-lockfile --production

# Copy source code
COPY . .

# Build the application
RUN bun run build

# Stage 2: Production
FROM oven/bun:1-alpine AS runner

WORKDIR /app

# Copy built assets from builder
COPY --from=builder /app/dist ./dist
COPY --from=builder /app/package.json ./

# Install only production dependencies needed for runtime
RUN bun install --frozen-lockfile --production

# Expose port
EXPOSE 3000

# Run the preview server
CMD ["bun", "run", "preview"]
