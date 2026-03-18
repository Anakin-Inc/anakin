FROM golang:1.25 AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /server ./cmd/server

# Install Playwright driver (downloads node + JS bundles)
RUN PLAYWRIGHT_SKIP_BROWSER_DOWNLOAD=1 \
    go run github.com/playwright-community/playwright-go/cmd/playwright@v0.5700.1 install

FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates && \
    rm -rf /var/lib/apt/lists/*
RUN useradd -m appuser

# Copy Playwright driver (node + JS)
COPY --from=builder /root/.cache/ms-playwright-go/ /home/appuser/.cache/ms-playwright-go/
RUN chown -R appuser:appuser /home/appuser/.cache/

USER appuser
COPY --from=builder /server /usr/local/bin/server

EXPOSE 8080
CMD ["server"]
