#!/usr/bin/env python3
"""
Mock cloud metadata and metrics service for testing SSRF and metrics refresh.
Listens on ports 80 (metadata) and 9090 (metrics).
"""
import http.server
import json
import threading

METADATA_RESPONSES = {
    "/": "latest\n",
    "/latest": "meta-data\n",
    "/latest/meta-data": "iam/\ninstance-id\nlocal-ipv4\nplacement/\n",
    "/latest/meta-data/": "iam/\ninstance-id\nlocal-ipv4\nplacement/\n",
    "/latest/meta-data/instance-id": "i-0987654321fedcba0\n",
    "/latest/meta-data/local-ipv4": "10.0.1.42\n",
    "/latest/meta-data/placement/availability-zone": "us-west-2a\n",
    "/latest/meta-data/iam/security-credentials": "dataflow-production-role\n",
    "/latest/meta-data/iam/security-credentials/": "dataflow-production-role\n",
    "/latest/meta-data/iam/security-credentials/dataflow-production-role": json.dumps({
        "Code": "Success",
        "LastUpdated": "2026-10-30T10:00:00Z",
        "Type": "AWS-HMAC",
        "AccessKeyId": "ASIAIOSFODNN7EXAMPLE",
        "SecretAccessKey": "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
        "Token": "AQoDYXdzEJr1111111111111111111111111111111111111111111111111111",
        "Expiration": "2026-10-30T16:00:00Z"
    }, indent=2) + "\n",
}

METRICS_RESPONSES = {
    "/api/v1/status": json.dumps({
        "status": "healthy",
        "metrics_version": "v1.4.0",
        "uptime_seconds": 12840,
        "nodes": 4,
        "cpu_usage_percent": 14.2,
        "memory_usage_percent": 41.8
    }) + "\n",
    "/metrics": "# HELP dataflow_jobs_total Total dataflow jobs\n# TYPE dataflow_jobs_total counter\ndataflow_jobs_total 42\n"
}

class MockHandler(http.server.BaseHTTPRequestHandler):
    def do_GET(self):
        port = self.server.server_port
        path = self.path.split("?")[0]
        store = METRICS_RESPONSES if port == 9090 else METADATA_RESPONSES

        if path in store:
            content = store[path].encode("utf-8")
            self.send_response(200)
            if content.startswith(b"{"):
                self.send_header("Content-Type", "application/json")
            else:
                self.send_header("Content-Type", "text/plain")
            self.send_header("Content-Length", str(len(content)))
            self.end_headers()
            self.wfile.write(content)
        else:
            self.send_response(404)
            self.send_header("Content-Type", "text/plain")
            msg = b"404 Not Found\n"
            self.send_header("Content-Length", str(len(msg)))
            self.end_headers()
            self.wfile.write(msg)

    def log_message(self, format, *args):
        # Keep logs clean
        pass

def run_server(port):
    server = http.server.HTTPServer(("0.0.0.0", port), MockHandler)
    server.serve_forever()

if __name__ == "__main__":
    t = threading.Thread(target=run_server, args=(9090,), daemon=True)
    t.start()
    run_server(80)
