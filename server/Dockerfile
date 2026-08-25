FROM golang:1.25-alpine AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /server ./cmd/server

FROM alpine:3.20
WORKDIR /app
COPY --from=builder /server .
VOLUME ["/app/data"]
EXPOSE 8080
ENV DB_DSN="file:/app/data/elements.db"
CMD ["/server"]
