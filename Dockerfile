# Stage 1: Build frontend
FROM node:20-alpine AS frontend
WORKDIR /app/ui
COPY ui/package.json ui/package-lock.json ./
RUN npm ci
COPY ui/ ./
RUN npm run build

# Stage 2: Build backend
FROM golang:1.22-alpine AS backend
RUN apk add --no-cache gcc musl-dev
WORKDIR /app
COPY go.mod ./
COPY . .
RUN go mod tidy
COPY --from=frontend /app/ui/build ./internal/server/static
RUN CGO_ENABLED=1 go build -o mediaforge ./cmd/mediaforge

# Stage 3: Runtime
FROM alpine:3.19
RUN apk add --no-cache ca-certificates
COPY --from=backend /app/mediaforge /usr/local/bin/mediaforge
EXPOSE 9876
ENTRYPOINT ["mediaforge"]
