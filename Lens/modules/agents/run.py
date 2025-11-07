#!/usr/bin/env python
"""Entry point for running the GPU Usage Analysis Agent API."""

import sys
import os

# Ensure /app is in the Python path
sys.path.insert(0, '/app')
os.chdir('/app')

if __name__ == "__main__":
    import uvicorn
    uvicorn.run(
        "api.main:app",
        host="0.0.0.0",
        port=8001,
        log_level="info"
    )

