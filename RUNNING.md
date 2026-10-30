# Running DataFlow Pro

This guide outlines how to build, run, and verify the DataFlow Pro application suite using Docker Compose.

---

## Prerequisites

- **Docker** 20.10+
- **Docker Compose** v2+ (or `docker-compose`)
- **Ports available on 127.0.0.1:**
  - `3000` (Frontend Dashboard)
  - `5000` (Data Service - Python)
  - `8080` (Analytics Engine - Java)
  - `8443` (API Gateway - Go, HTTPS)
  - `3306` (Analytics Database - MySQL)

---

## Quick Start

1. **Build and start all services:**
   ```bash
   docker compose up --build -d
   ```

2. **Check container status and health:**
   ```bash
   docker compose ps
   ```
   Wait until all services report healthy / running.

3. **View logs:**
   ```bash
   docker compose logs -f
   ```

4. **Stop the stack:**
   ```bash
   docker compose down
   ```

---

## Services & Ports

| Service | Host Port | Protocol | Description |
|---|---|---|---|
| **API Gateway** | `https://127.0.0.1:8443` | HTTPS (self-signed) | Primary REST routing, auth, proxies |
| **Frontend Dashboard** | `http://127.0.0.1:3000` | HTTP | Web UI and visualization |
| **Data Service** | `http://127.0.0.1:5000` | HTTP | Python ETL, model serving, engine |
| **Analytics Engine** | `http://127.0.0.1:8080` | HTTP | Java query engine, XML, encryption |
| **MySQL Database** | `127.0.0.1:3306` | MySQL | Internal analytics database |

---

## Health Checks & Sanity Verification

You can run these basic probes to verify each service is up:

### 1. API Gateway (HTTPS)
```bash
curl -k -s https://127.0.0.1:8443/api/v1/health
# Expected: {"status":"healthy","version":"2.4.1"}
```

### 2. Frontend Dashboard
```bash
curl -s -I http://127.0.0.1:3000/
# Expected: HTTP/1.1 200 OK
```

### 3. Data Service (Python)
```bash
curl -s http://127.0.0.1:5000/api/v1/health
# Expected: {"status":"healthy","version":"2.4.1"}
```

### 4. Analytics Engine (Java)
```bash
curl -s "http://127.0.0.1:8080/api/v1/datasets/search?q=rev"
# Expected: JSON array of matching datasets
```

### 5. Gateway Reverse Proxy Routing
```bash
# Proxy to Python
curl -k -s "https://127.0.0.1:8443/api/v1/datasets/by-owner?owner=alice"

# Proxy to Java
curl -k -s "https://127.0.0.1:8443/api/v1/users/lookup?username=admin"
```
