FROM python:3.12-slim
RUN pip install --no-cache-dir aiohttp PyJWT cryptography
COPY server.py /app/server.py
ENTRYPOINT ["python", "/app/server.py"]
