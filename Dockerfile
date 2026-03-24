# Stage 1 — Frontend build
FROM node:22-alpine AS frontend
WORKDIR /app/frontend
COPY frontend/package.json frontend/package-lock.json ./
RUN npm ci
COPY frontend/ ./
RUN npm run build

# Stage 2 — Backend build
FROM golang:1.25-alpine AS backend
RUN apk add --no-cache git
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=frontend /app/frontend/dist ./frontend/dist
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /server ./cmd/server

# Stage 3 — Runtime
FROM alpine:3.21
RUN apk add --no-cache ca-certificates tzdata git bash
COPY --from=backend /server /server
COPY migrations/ /migrations/
EXPOSE 5110
CMD ["/server"]
