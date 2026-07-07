# Build stage
FROM golang:1.22-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o noroi ./cmd/server

# Final stage
FROM scratch
COPY --from=builder /app/noroi /noroi
USER 65532:65532
EXPOSE 8080
ENTRYPOINT ["/noroi"]
