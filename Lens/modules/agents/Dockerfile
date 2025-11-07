# GPU Usage Analysis Agent Dockerfile

FROM python:3.11-slim

# 设置工作目录
WORKDIR /app

# 安装系统依赖
RUN apt-get update && apt-get install -y \
    gcc \
    g++ \
    curl \
    && rm -rf /var/lib/apt/lists/*

# 复制依赖文件
COPY requirements.txt .

# 安装 Python 依赖
RUN pip install --no-cache-dir -r requirements.txt

# 复制应用代码
COPY gpu_usage_agent/ ./gpu_usage_agent/
COPY api/ ./api/
COPY config/ ./config/

# 设置环境变量
ENV PYTHONUNBUFFERED=1
ENV LENS_API_URL=http://localhost:8080
ENV LLM_PROVIDER=openai
ENV LLM_MODEL=gpt-4

# 暴露端口
EXPOSE 8001

# 健康检查
HEALTHCHECK --interval=30s --timeout=10s --start-period=5s --retries=3 \
    CMD curl -f http://localhost:8001/health || exit 1

# 启动命令
CMD ["python", "api/main.py"]

