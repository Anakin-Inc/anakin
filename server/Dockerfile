FROM golang:1.22-alpine AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /server ./cmd/server

FROM alpine:3.21
RUN apk add --no-cache ca-certificates && update-ca-certificates
RUN adduser -D appuser
USER appuser
COPY --from=builder /server /usr/local/bin/server

EXPOSE 8080
CMD ["server"]
