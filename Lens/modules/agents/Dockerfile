# GPU Usage Analysis Agent Dockerfile

FROM python:3.11-slim

# Set working directory
WORKDIR /app

# Install system dependencies
RUN apt-get update && apt-get install -y \
    gcc \
    g++ \
    curl \
    && rm -rf /var/lib/apt/lists/*

# Copy requirements file
COPY requirements.txt .

# Install Python dependencies
RUN pip install --no-cache-dir -r requirements.txt

# Copy root level files
COPY __init__.py ./
COPY llm_wrapper.py ./
COPY run.py ./

# Copy module directories
COPY gpu_usage_agent/ ./gpu_usage_agent/
COPY api/ ./api/
COPY cache/ ./cache/
COPY storage/ ./storage/

# Copy config directory - try different approach
# First create the directory
RUN mkdir -p /app/config

# Copy each file explicitly
COPY config/__init__.py ./config/
COPY config/loader.py ./config/
COPY config/config.yaml ./config/

# Verify all __init__.py files are present
RUN echo "=== Verifying copied files ===" && \
    echo "All __init__.py files:" && \
    find /app -name "__init__.py" -type f | sort && \
    echo "" && \
    echo "Config directory contents:" && \
    ls -lah /app/config/ && \
    echo "" && \
    echo "Config __init__.py file info:" && \
    if [ -f /app/config/__init__.py ]; then \
        echo "File exists!" && \
        cat /app/config/__init__.py; \
    else \
        echo "FILE NOT FOUND!" && \
        exit 1; \
    fi

# Set environment variables
ENV PYTHONUNBUFFERED=1
ENV PYTHONPATH=/app

# Expose port
EXPOSE 8001

# Health check
HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
    CMD curl -f http://localhost:8001/health || exit 1

# Start command - use run.py to ensure correct Python path
CMD ["python", "run.py"]
