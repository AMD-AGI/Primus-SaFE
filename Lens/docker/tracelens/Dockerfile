# TraceLens Container Image
# Provides on-demand trace analysis using Streamlit UI
#
# Build: docker build -t tracelens:latest .
# Run: docker run -p 8501:8501 -e TRACE_FILE_PATH=/path/to/trace tracelens:latest

FROM python:3.10-slim

LABEL maintainer="AMD AGI Team"
LABEL description="TraceLens - GPU Trace Analysis Tool with Streamlit UI"
LABEL version="1.0.0"

# Install system dependencies
RUN apt-get update && apt-get install -y --no-install-recommends \
    git \
    curl \
    && rm -rf /var/lib/apt/lists/*

# Create non-root user for security
RUN useradd -m -u 1000 -s /bin/bash tracelens

# Set working directory
WORKDIR /app

# Install Python dependencies
# TraceLens from GitHub + required packages for Streamlit UI
RUN pip install --no-cache-dir \
    torch>=2.0.0 \
    git+https://github.com/AMD-AGI/TraceLens.git \
    streamlit>=1.28.0 \
    openpyxl>=3.1.0 \
    plotly>=5.18.0 \
    requests>=2.31.0 \
    pandas>=2.0.0 \
    numpy>=1.24.0

# Copy application files
COPY analyze_trace.py /app/
COPY entrypoint.sh /app/

# Set proper permissions
RUN chown -R tracelens:tracelens /app && \
    chmod +x /app/entrypoint.sh

# Switch to non-root user
USER tracelens

# Expose Streamlit default port
EXPOSE 8501

# Health check for Streamlit
HEALTHCHECK --interval=30s --timeout=10s --start-period=60s --retries=3 \
    CMD curl -f http://localhost:8501/_stcore/health || exit 1

# Default environment variables
ENV STREAMLIT_SERVER_PORT=8501 \
    STREAMLIT_SERVER_HEADLESS=true \
    STREAMLIT_SERVER_ENABLE_CORS=false \
    STREAMLIT_SERVER_ENABLE_XSRF_PROTECTION=false

# Entry point
ENTRYPOINT ["/app/entrypoint.sh"]

