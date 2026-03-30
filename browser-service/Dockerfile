FROM python:3.11-slim

RUN apt-get update && apt-get install -y --no-install-recommends \
    curl \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app

COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt

# Fetch Camoufox browser binary
RUN python3 -m camoufox fetch

# Install Firefox system dependencies
RUN playwright install-deps firefox 2>/dev/null || \
    apt-get update && apt-get install -y --no-install-recommends \
    fonts-liberation fonts-noto-color-emoji \
    libgtk-3-0 libdbus-glib-1-2 libxt6 libx11-xcb1 \
    libasound2t64 libxcomposite1 libxdamage1 libxrandr2 \
    && rm -rf /var/lib/apt/lists/*

COPY server.py .

EXPOSE 9222 8080

CMD ["python", "server.py"]
