#!/usr/bin/env python3
# Copyright (C) 2025-2026, Advanced Micro Devices, Inc. All rights reserved.
# See LICENSE for license information.

"""Simple Mock Agent - just prints received requests"""
from http.server import HTTPServer, BaseHTTPRequestHandler
import json

class Handler(BaseHTTPRequestHandler):
    def do_GET(self):
        print(f"[GET] {self.path}")
        if self.path == '/health':
            self.send_response(200)
            self.send_header('Content-Type', 'application/json')
            self.end_headers()
            self.wfile.write(b'{"status":"healthy"}')
        else:
            self.send_error(404)
    
    def do_POST(self):
        content_length = int(self.headers.get('Content-Length', 0))
        body = self.rfile.read(content_length).decode()
        print(f"\n{'='*60}")
        print(f"[POST] {self.path}")
        print(f"Headers: {dict(self.headers)}")
        print(f"Body: {body}")
        print(f"{'='*60}\n")
        
        self.send_response(200)
        self.send_header('Content-Type', 'application/json')
        self.end_headers()
        result = {"status": "success", "code": 0, "message": "Mock response", "payload": {"received": True}}
        self.wfile.write(json.dumps(result).encode())

if __name__ == '__main__':
    server = HTTPServer(('0.0.0.0', 8002), Handler)
    print("Mock Agent listening on port 8002...")
    server.serve_forever()

