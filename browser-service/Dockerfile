FROM python:3.11-slim

# Install system dependencies for Playwright Chromium
RUN apt-get update && apt-get install -y --no-install-recommends \
    curl \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app

# Install Python dependencies
COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt

# Install Playwright browsers (chromium only)
# Install deps separately to handle missing font packages on newer Debian
RUN playwright install-deps chromium 2>/dev/null || \
    apt-get update && apt-get install -y --no-install-recommends \
    fonts-liberation fonts-noto-color-emoji fonts-unifont \
    libasound2t64 libatk-bridge2.0-0 libatk1.0-0 libcairo2 libcups2 \
    libdbus-1-3 libdrm2 libgbm1 libglib2.0-0 libnspr4 libnss3 \
    libpango-1.0-0 libx11-6 libxcb1 libxcomposite1 libxdamage1 \
    libxext6 libxfixes3 libxkbcommon0 libxrandr2 \
    && rm -rf /var/lib/apt/lists/*
RUN playwright install chromium

# Copy application
COPY server.py .

EXPOSE 9222 8080

CMD ["python", "server.py"]
