#!/usr/bin/env python3
"""
Mock AI Agent for End-to-End Testing (Phase 1-5)

This mock agent:
1. Registers itself with the AI Gateway
2. Provides a /health endpoint for health checks
3. Polls for tasks from PostgreSQL and processes them
4. Returns mock results

Usage:
    python3 mock_agent.py [--port 8002] [--gateway http://localhost:8003] [--db-dsn ...]
"""

import argparse
import json
import logging
import signal
import sys
import threading
import time
from datetime import datetime, timezone
from http.server import HTTPServer, BaseHTTPRequestHandler
from typing import Optional

import psycopg2
import psycopg2.extras
import requests

logging.basicConfig(
    level=logging.INFO,
    format='%(asctime)s - %(name)s - %(levelname)s - %(message)s'
)
logger = logging.getLogger('mock-agent')


class MockAgentConfig:
    def __init__(self, args):
        self.name = args.name
        self.port = args.port
        self.gateway_url = args.gateway
        self.db_dsn = args.db_dsn
        self.topics = args.topics.split(',')
        self.poll_interval = args.poll_interval
        self.endpoint = f"http://localhost:{self.port}"


class TaskPoller:
    """Polls for tasks from PostgreSQL and processes them"""
    
    def __init__(self, config: MockAgentConfig):
        self.config = config
        self.running = False
        self.conn: Optional[psycopg2.connection] = None
        
    def connect(self):
        """Connect to PostgreSQL"""
        self.conn = psycopg2.connect(self.config.db_dsn)
        self.conn.autocommit = False
        logger.info("Connected to PostgreSQL")
        
    def poll_and_process(self):
        """Poll for a pending task and process it"""
        if not self.conn:
            return
            
        try:
            with self.conn.cursor(cursor_factory=psycopg2.extras.DictCursor) as cur:
                # Use SELECT FOR UPDATE SKIP LOCKED pattern
                cur.execute("""
                    SELECT id, topic, input_payload, context
                    FROM ai_tasks
                    WHERE status = 'pending'
                      AND topic = ANY(%s)
                    ORDER BY priority DESC, created_at ASC
                    LIMIT 1
                    FOR UPDATE SKIP LOCKED
                """, (self.config.topics,))
                
                row = cur.fetchone()
                if not row:
                    self.conn.rollback()
                    return
                    
                task_id = row['id']
                topic = row['topic']
                input_payload = row['input_payload']
                
                logger.info(f"Claimed task {task_id} for topic {topic}")
                
                # Update status to processing
                cur.execute("""
                    UPDATE ai_tasks
                    SET status = 'processing',
                        agent_id = %s,
                        started_at = NOW()
                    WHERE id = %s
                """, (self.config.name, task_id))
                
                self.conn.commit()
                
                # Process the task (mock processing)
                try:
                    result = self.process_task(topic, input_payload)
                    
                    # Mark as completed
                    with self.conn.cursor() as cur2:
                        cur2.execute("""
                            UPDATE ai_tasks
                            SET status = 'completed',
                                output_payload = %s,
                                completed_at = NOW()
                            WHERE id = %s
                        """, (json.dumps(result), task_id))
                        self.conn.commit()
                        
                    logger.info(f"Completed task {task_id}")
                    
                except Exception as e:
                    logger.error(f"Failed to process task {task_id}: {e}")
                    with self.conn.cursor() as cur2:
                        cur2.execute("""
                            UPDATE ai_tasks
                            SET status = 'failed',
                                error_message = %s,
                                error_code = 500,
                                completed_at = NOW()
                            WHERE id = %s
                        """, (str(e), task_id))
                        self.conn.commit()
                        
        except Exception as e:
            logger.error(f"Poll error: {e}")
            self.conn.rollback()
            
    def process_task(self, topic: str, input_payload: dict) -> dict:
        """Process a task and return mock result"""
        logger.info(f"Processing task for topic: {topic}")
        
        # Simulate processing time
        time.sleep(0.5)
        
        # Generate mock result based on topic
        if topic == "alert.advisor.aggregate-workloads":
            return {
                "status": "success",
                "code": 0,
                "message": "Workloads aggregated successfully",
                "payload": {
                    "groups": [
                        {
                            "group_id": "mock-group-1",
                            "name": "PostgreSQL Cluster",
                            "component_type": "postgresql",
                            "category": "database",
                            "members": ["workload-1", "workload-2"],
                            "aggregation_reason": "Same image prefix and labels",
                            "confidence": 0.95
                        }
                    ],
                    "ungrouped": [],
                    "stats": {
                        "total_workloads": 2,
                        "grouped_workloads": 2,
                        "total_groups": 1
                    }
                }
            }
        elif topic == "alert.advisor.generate-suggestions":
            return {
                "status": "success",
                "code": 0,
                "message": "Suggestions generated successfully",
                "payload": {
                    "suggestions": [
                        {
                            "suggestion_id": "sug-1",
                            "rule_name": "PostgreSQLHighConnections",
                            "description": "Alert when PostgreSQL connection count is high",
                            "category": "capacity",
                            "severity": "warning",
                            "prometheus_rule": {
                                "expr": "pg_stat_activity_count > 100",
                                "for": "5m",
                                "labels": {"severity": "warning"},
                                "annotations": {"summary": "High PostgreSQL connections"}
                            },
                            "rationale": "Monitor connection pool exhaustion",
                            "confidence": 0.9,
                            "priority": 1
                        }
                    ],
                    "coverage_analysis": {
                        "existing_coverage": ["availability"],
                        "missing_coverage": ["performance", "capacity"],
                        "coverage_score": 0.33
                    }
                }
            }
        elif topic == "scan.identify-component":
            return {
                "status": "success",
                "code": 0,
                "message": "Component identified successfully",
                "payload": {
                    "component_type": "redis",
                    "confidence": 0.85,
                    "evidence": [
                        "Image contains 'redis'",
                        "Port 6379 exposed"
                    ]
                }
            }
        else:
            return {
                "status": "success",
                "code": 0,
                "message": f"Mock result for topic: {topic}",
                "payload": {
                    "mock": True,
                    "topic": topic,
                    "processed_at": datetime.now(timezone.utc).isoformat()
                }
            }
            
    def run(self):
        """Run the polling loop"""
        self.running = True
        self.connect()
        
        logger.info(f"Starting task poller (interval: {self.config.poll_interval}s)")
        
        while self.running:
            self.poll_and_process()
            time.sleep(self.config.poll_interval)
            
    def stop(self):
        """Stop the polling loop"""
        self.running = False
        if self.conn:
            self.conn.close()


class AgentHTTPHandler(BaseHTTPRequestHandler):
    """HTTP handler for the mock agent"""
    
    def log_message(self, format, *args):
        logger.debug(f"HTTP: {format % args}")
        
    def do_GET(self):
        if self.path == '/health':
            self.send_response(200)
            self.send_header('Content-Type', 'application/json')
            self.end_headers()
            response = {
                "status": "healthy",
                "name": self.server.config.name,
                "timestamp": datetime.now(timezone.utc).isoformat()
            }
            self.wfile.write(json.dumps(response).encode())
        else:
            self.send_error(404)
            
    def do_POST(self):
        if self.path == '/invoke':
            # Sync invocation endpoint (not used in async flow, but good to have)
            content_length = int(self.headers['Content-Length'])
            body = self.rfile.read(content_length)
            request = json.loads(body)
            
            topic = request.get('topic', '')
            payload = request.get('payload', {})
            
            result = TaskPoller(self.server.config).process_task(topic, payload)
            
            self.send_response(200)
            self.send_header('Content-Type', 'application/json')
            self.end_headers()
            self.wfile.write(json.dumps(result).encode())
        else:
            self.send_error(404)


class MockAgent:
    """Main mock agent class"""
    
    def __init__(self, config: MockAgentConfig):
        self.config = config
        self.poller: Optional[TaskPoller] = None
        self.http_server: Optional[HTTPServer] = None
        self.running = False
        
    def register(self):
        """Register with AI Gateway"""
        url = f"{self.config.gateway_url}/api/v1/ai/agents/register"
        payload = {
            "name": self.config.name,
            "endpoint": self.config.endpoint,
            "topics": self.config.topics,
            "health_check_path": "/health",
            "timeout_secs": 60,
            "metadata": {
                "version": "1.0.0",
                "type": "mock"
            }
        }
        
        try:
            resp = requests.post(url, json=payload, timeout=10)
            if resp.status_code == 200:
                logger.info(f"Registered with gateway: {resp.json()}")
                return True
            else:
                logger.error(f"Registration failed: {resp.status_code} - {resp.text}")
                return False
        except Exception as e:
            logger.error(f"Registration error: {e}")
            return False
            
    def unregister(self):
        """Unregister from AI Gateway"""
        url = f"{self.config.gateway_url}/api/v1/ai/agents/{self.config.name}"
        
        try:
            resp = requests.delete(url, timeout=10)
            if resp.status_code == 200:
                logger.info("Unregistered from gateway")
            else:
                logger.warning(f"Unregister response: {resp.status_code}")
        except Exception as e:
            logger.warning(f"Unregister error: {e}")
            
    def start(self):
        """Start the mock agent"""
        self.running = True
        
        # Start HTTP server
        self.http_server = HTTPServer(('0.0.0.0', self.config.port), AgentHTTPHandler)
        self.http_server.config = self.config
        http_thread = threading.Thread(target=self.http_server.serve_forever)
        http_thread.daemon = True
        http_thread.start()
        logger.info(f"HTTP server started on port {self.config.port}")
        
        # Register with gateway
        if not self.register():
            logger.warning("Failed to register, but continuing...")
            
        # Start task poller
        self.poller = TaskPoller(self.config)
        poller_thread = threading.Thread(target=self.poller.run)
        poller_thread.daemon = True
        poller_thread.start()
        
        logger.info("Mock agent started successfully")
        
    def stop(self):
        """Stop the mock agent"""
        logger.info("Stopping mock agent...")
        self.running = False
        
        if self.poller:
            self.poller.stop()
            
        if self.http_server:
            self.http_server.shutdown()
            
        self.unregister()
        logger.info("Mock agent stopped")


def main():
    parser = argparse.ArgumentParser(description='Mock AI Agent for E2E Testing')
    parser.add_argument('--name', default='mock-agent', help='Agent name')
    parser.add_argument('--port', type=int, default=8002, help='HTTP port')
    parser.add_argument('--gateway', default='http://localhost:8003', help='AI Gateway URL')
    parser.add_argument('--db-dsn', default='host=localhost port=5432 dbname=lens user=lens password=lens',
                        help='PostgreSQL connection string')
    parser.add_argument('--topics', default='alert.advisor.aggregate-workloads,alert.advisor.generate-suggestions,scan.identify-component',
                        help='Comma-separated list of topics to handle')
    parser.add_argument('--poll-interval', type=float, default=1.0, help='Task poll interval in seconds')
    
    args = parser.parse_args()
    config = MockAgentConfig(args)
    
    agent = MockAgent(config)
    
    # Handle signals
    def signal_handler(sig, frame):
        agent.stop()
        sys.exit(0)
        
    signal.signal(signal.SIGINT, signal_handler)
    signal.signal(signal.SIGTERM, signal_handler)
    
    # Start agent
    agent.start()
    
    # Keep running
    logger.info("Press Ctrl+C to stop")
    while agent.running:
        time.sleep(1)


if __name__ == '__main__':
    main()

