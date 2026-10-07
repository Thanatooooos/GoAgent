FROM debian:bookworm-slim

RUN apt-get update && apt-get install -y --no-install-recommends \
    python3 tesseract-ocr tesseract-ocr-eng tesseract-ocr-chi-sim \
    && rm -rf /var/lib/apt/lists/*

COPY docker/ocr/server.py /opt/ocr/server.py

EXPOSE 8080
CMD ["python3", "/opt/ocr/server.py"]
