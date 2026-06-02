FROM python:3.13-slim AS builder

WORKDIR /build

# hadolint ignore=DL3008
RUN apt-get update && apt-get install -y --no-install-recommends \
    build-essential \
    && rm -rf /var/lib/apt/lists/*

COPY requirements.txt .
RUN pip install --no-cache-dir --prefix=/install -r requirements.txt

# ── Runtime ──────────────────────────────────────────────────────────────────

FROM python:3.13-slim

# hadolint ignore=DL3008
RUN apt-get update && apt-get install -y --no-install-recommends \
    wget \
    && rm -rf /var/lib/apt/lists/* \
    && useradd -m -u 1000 app

WORKDIR /app
COPY --from=builder /install /usr/local

COPY --chown=app:app src/ ./src/
# data/dictionary.db must be pre-built (CI runs scripts/import_jmdict.py
# before `docker build` — see .github/workflows/deploy.yml).
COPY --chown=app:app data/dictionary.db ./data/dictionary.db

ENV DICT_DB_PATH=/app/data/dictionary.db \
    PORT=8001 \
    PYTHONUNBUFFERED=1

USER app
EXPOSE 8001

HEALTHCHECK --interval=30s --timeout=5s --retries=3 \
  CMD wget -qO- http://localhost:8001/health || exit 1

CMD ["uvicorn", "src.app:app", "--host", "0.0.0.0", "--port", "8001", "--workers", "2"]
