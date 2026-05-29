FROM python:3.13-slim

WORKDIR /app

# Install system dependencies for SudachiPy
RUN apt-get update && apt-get install -y --no-install-recommends \
    build-essential \
    && rm -rf /var/lib/apt/lists/*

COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt

COPY src/ ./src/
COPY data/dictionary.db ./data/dictionary.db

ENV DICT_DB_PATH=/app/data/dictionary.db
ENV NLP_INTERNAL_SECRET=""
ENV PORT=8001

EXPOSE 8001

CMD ["uvicorn", "src.app:app", "--host", "0.0.0.0", "--port", "8001", "--workers", "2"]
