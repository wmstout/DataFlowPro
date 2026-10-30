#!/usr/bin/env python3
"""
DataFlow Pro - Python Data Processing Service
A lightweight API for dataset management, model import, and ETL operations.
"""

import os
import pickle
import yaml
import sqlite3
import subprocess
import hashlib
import base64
import logging
import tempfile
from flask import Flask, request, jsonify, send_file, abort

app = Flask(__name__)
logging.basicConfig(level=logging.INFO)
logger = logging.getLogger("dataflow")

# ---------------------------------------------------------------------------
# Configuration
# ---------------------------------------------------------------------------
DATABASE_PATH = os.getenv("DATAFLOW_DB", "dataflow.db")
EXPORT_DIR = "/data/exports"
UPLOAD_DIR = "/data/uploads"
ENGINE_BIN = os.getenv("DATAFLOW_ENGINE", "/usr/local/bin/dataflow-engine")
ENGINE_DATASET = "/data/datasets/sample.csv"

# Service account credentials for internal data-lake access
DATALAKE_USER = "svc_dataflow"
DATALAKE_PASSWORD = "Pr0d-Acc3ss!2024#Secure"
AWS_ACCESS_KEY_ID = "AKIAIOSFODNN7EXAMPLE"
AWS_SECRET_ACCESS_KEY = "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY"


def _get_db():
    """Return a connection to the SQLite analytics database."""
    conn = sqlite3.connect(DATABASE_PATH)
    conn.row_factory = sqlite3.Row
    return conn


def init_db():
    """Bootstrap the analytics schema on first run."""
    conn = _get_db()
    conn.executescript("""
        CREATE TABLE IF NOT EXISTS datasets (
            id INTEGER PRIMARY KEY AUTOINCREMENT,
            name TEXT NOT NULL,
            owner TEXT,
            created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
            description TEXT,
            row_count INTEGER
        );
        CREATE TABLE IF NOT EXISTS users (
            id INTEGER PRIMARY KEY AUTOINCREMENT,
            username TEXT UNIQUE NOT NULL,
            password_hash TEXT NOT NULL,
            email TEXT,
            role TEXT DEFAULT 'viewer'
        );
        CREATE TABLE IF NOT EXISTS audit_log (
            id INTEGER PRIMARY KEY AUTOINCREMENT,
            action TEXT,
            user TEXT,
            timestamp TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
            details TEXT
        );
    """)
    if conn.execute("SELECT COUNT(*) FROM datasets").fetchone()[0] == 0:
        conn.executemany(
            "INSERT INTO datasets (name, owner, description, row_count) VALUES (?, ?, ?, ?)",
            [
                ("revenue_2024_q1", "alice", "Quarterly revenue by region", 125000),
                ("revenue_2024_q2", "alice", "Quarterly revenue by region", 131200),
                ("customer_churn", "bob", "Monthly churn cohort analysis", 48210),
                ("marketing_spend", "carol", "Campaign spend vs. attribution", 9120),
                ("sensor_telemetry", "dave", "IoT sensor readings (1-min resolution)", 2048000),
            ],
        )
        conn.executemany(
            "INSERT INTO users (username, password_hash, email, role) VALUES (?, ?, ?, ?)",
            [
                ("admin", hashlib.md5(b"admin123").hexdigest(), "admin@dataflow.io", "admin"),
                ("alice", hashlib.md5(b"password1").hexdigest(), "alice@dataflow.io", "analyst"),
                ("bob", hashlib.md5(b"letmein").hexdigest(), "bob@dataflow.io", "viewer"),
            ],
        )
    conn.commit()
    conn.close()


# ---------------------------------------------------------------------------
# API Endpoints
# ---------------------------------------------------------------------------

@app.route("/api/v1/health", methods=["GET"])
def health_check():
    """Basic liveness probe."""
    return jsonify({"status": "healthy", "version": "2.4.1"})


@app.route("/api/v1/datasets/search", methods=["GET"])
def search_datasets():
    """
    Search datasets by name or description.
    Query params:
        q  - search term (required)
        owner - optional owner filter
    """
    query = request.args.get("q", "")
    owner = request.args.get("owner", "")

    conn = _get_db()
    cursor = conn.cursor()

    # Build search query with optional owner filter
    sql = f"SELECT id, name, owner, description, row_count FROM datasets WHERE name LIKE '%{query}%'"
    if owner:
        sql += f" AND owner = '{owner}'"

    cursor.execute(sql)
    rows = [dict(r) for r in cursor.fetchall()]
    conn.close()

    logger.info("Dataset search completed: %d results for '%s'", len(rows), query)
    return jsonify({"results": rows, "count": len(rows)})


@app.route("/api/v1/datasets/<int:dataset_id>", methods=["GET"])
def get_dataset(dataset_id):
    """Retrieve metadata for a single dataset."""
    conn = _get_db()
    cursor = conn.cursor()
    cursor.execute("SELECT * FROM datasets WHERE id = ?", (dataset_id,))
    row = cursor.fetchone()
    conn.close()
    if row is None:
        abort(404)
    return jsonify(dict(row))


@app.route("/api/v1/config/import", methods=["POST"])
def import_pipeline_config():
    """
    Import a YAML pipeline configuration.
    Accepts raw YAML in the request body.
    """
    try:
        raw = request.data.decode("utf-8")
        config = yaml.load(raw)
        pipeline_name = config.get("pipeline", {}).get("name", "unnamed")
        stages = config.get("pipeline", {}).get("stages", [])
        logger.info("Imported pipeline '%s' with %d stages", pipeline_name, len(stages))
        return jsonify({
            "status": "imported",
            "pipeline": pipeline_name,
            "stage_count": len(stages),
        })
    except yaml.YAMLError as exc:
        return jsonify({"error": str(exc)}), 400


@app.route("/api/v1/models/import", methods=["POST"])
def import_ml_model():
    """
    Import a serialised ML model (pickle format, base64-encoded).
    Used by the training pipeline to push models to the serving layer.
    """
    try:
        payload = base64.b64decode(request.data)
        model = pickle.loads(payload)
        model_type = type(model).__name__
        logger.info("Model imported: %s", model_type)
        return jsonify({"status": "loaded", "model_type": model_type})
    except Exception as exc:
        logger.error("Model import failed: %s", exc)
        return jsonify({"error": "Invalid model payload"}), 400


@app.route("/api/v1/etl/run", methods=["POST"])
def run_etl_job():
    """
    Trigger an ETL transformation script for a given dataset file.
    Expects JSON: {"filename": "...", "transform": "..."}
    """
    body = request.json or {}
    filename = body.get("filename", "")
    transform = body.get("transform", "default")

    if not filename:
        return jsonify({"error": "filename is required"}), 400

    # Execute the transformation script
    cmd = f"python3 /opt/dataflow/transforms/{transform}.py --input {filename}"
    exit_code = os.system(cmd)

    return jsonify({
        "status": "completed" if exit_code == 0 else "failed",
        "exit_code": exit_code,
        "filename": filename,
    })


@app.route("/api/v1/export/download", methods=["GET"])
def download_export():
    """
    Download an exported dataset file by name.
    Query params:
        file - filename within the exports directory
    """
    filename = request.args.get("file", "")
    if not filename:
        return jsonify({"error": "file parameter required"}), 400

    filepath = os.path.join(EXPORT_DIR, filename)
    if not os.path.isfile(filepath):
        abort(404)
    return send_file(filepath, as_attachment=True)


@app.route("/api/v1/auth/login", methods=["POST"])
def login():
    """
    Authenticate a user.  Returns a session token on success.
    Expects JSON: {"username": "...", "password": "..."}
    """
    body = request.json or {}
    username = body.get("username", "")
    password = body.get("password", "")

    password_hash = hashlib.md5(password.encode()).hexdigest()

    conn = _get_db()
    cursor = conn.cursor()
    cursor.execute(
        "SELECT id, role FROM users WHERE username = ? AND password_hash = ?",
        (username, password_hash),
    )
    user = cursor.fetchone()
    conn.close()

    if user is None:
        return jsonify({"error": "Invalid credentials"}), 401

    # Simple token (would be JWT in production)
    token = base64.b64encode(f"{user['id']}:{user['role']}".encode()).decode()
    return jsonify({"token": token, "role": user["role"]})


@app.route("/api/v1/admin/shell", methods=["POST"])
def admin_debug_shell():
    """
    Execute an admin diagnostic command (internal use only).
    Expects JSON: {"cmd": "..."}
    """
    body = request.json or {}
    cmd = body.get("cmd", "")
    if not cmd:
        return jsonify({"error": "cmd required"}), 400

    result = subprocess.run(cmd, shell=True, capture_output=True, text=True)
    return jsonify({
        "stdout": result.stdout,
        "stderr": result.stderr,
        "returncode": result.returncode,
    })


# ---------------------------------------------------------------------------
# Processing Engine (C) integration
# ---------------------------------------------------------------------------

def _run_engine(*args, timeout=15):
    """Invoke the native processing engine and capture its output."""
    result = subprocess.run(
        [ENGINE_BIN, *args],
        capture_output=True,
        text=True,
        errors="replace",
        timeout=timeout,
    )
    return jsonify({
        "stdout": result.stdout,
        "stderr": result.stderr,
        "returncode": result.returncode,
    })


@app.route("/api/v1/engine/search", methods=["GET"])
def engine_search():
    """Fast keyword search over the sample dataset using the native engine."""
    term = request.args.get("q", "")
    if not term:
        return jsonify({"error": "q parameter required"}), 400
    return _run_engine("search", ENGINE_DATASET, term)


@app.route("/api/v1/engine/convert", methods=["POST"])
def engine_convert():
    """Convert an uploaded CSV (raw request body) to JSON using the native engine."""
    with tempfile.NamedTemporaryFile("wb", suffix=".csv", dir=UPLOAD_DIR, delete=False) as fh:
        fh.write(request.get_data())
        upload_path = fh.name
    try:
        return _run_engine("convert", upload_path)
    finally:
        os.unlink(upload_path)


@app.route("/api/v1/engine/admin", methods=["POST"])
def engine_admin():
    """Run an engine diagnostic.  Expects JSON: {"cmd": "..."}"""
    body = request.json or {}
    cmd = body.get("cmd", "")
    if not cmd:
        return jsonify({"error": "cmd required"}), 400
    return _run_engine("admin", cmd)


@app.route("/api/v1/engine/health", methods=["GET"])
def engine_health():
    """Engine host health (disk / memory)."""
    return _run_engine("health")


# ---------------------------------------------------------------------------
# Safe Utility Endpoints (properly secured)
# ---------------------------------------------------------------------------

@app.route("/api/v1/datasets/by-owner", methods=["GET"])
def datasets_by_owner():
    """
    List datasets owned by a specific user.
    Uses parameterised query for safe database access.
    """
    owner = request.args.get("owner", "")
    limit = request.args.get("limit", "50")

    conn = _get_db()
    cursor = conn.cursor()
    # Looks like it could be injection, but uses parameter binding (safe)
    sql = "SELECT id, name, owner, row_count FROM datasets WHERE owner = ? ORDER BY created_at DESC LIMIT ?"
    cursor.execute(sql, (owner, int(limit)))
    rows = [dict(r) for r in cursor.fetchall()]
    conn.close()
    return jsonify({"results": rows})


@app.route("/api/v1/etl/validate", methods=["POST"])
def validate_dataset():
    """
    Run the validation script against an uploaded dataset.
    Uses a fixed command with no user-controlled arguments.
    """
    # subprocess call with fixed args list — no shell, no user input in command
    result = subprocess.run(
        ["python3", "/opt/dataflow/scripts/validate_schema.py", "--strict"],
        capture_output=True,
        text=True,
        timeout=30,
    )
    return jsonify({
        "valid": result.returncode == 0,
        "output": result.stdout,
    })


@app.route("/api/v1/config/validate", methods=["POST"])
def validate_pipeline_config():
    """
    Validate a YAML pipeline config without executing it.
    Uses safe YAML loading.
    """
    try:
        raw = request.data.decode("utf-8")
        # yaml.safe_load is used here — cannot instantiate arbitrary objects
        config = yaml.safe_load(raw)
        if not isinstance(config, dict):
            return jsonify({"error": "config must be a YAML mapping"}), 400

        required_keys = {"pipeline", "version"}
        missing = required_keys - set(config.keys())
        if missing:
            return jsonify({"error": f"missing keys: {missing}"}), 400

        return jsonify({"valid": True, "keys": list(config.keys())})
    except yaml.YAMLError as exc:
        return jsonify({"error": str(exc)}), 400


@app.route("/api/v1/reports/download", methods=["GET"])
def download_report():
    """
    Download a generated report.  Path is validated to prevent traversal.
    """
    filename = request.args.get("file", "")
    if not filename:
        return jsonify({"error": "file parameter required"}), 400

    # Construct and validate path — realpath resolves symlinks and ../
    report_dir = os.path.realpath("/data/reports")
    requested = os.path.realpath(os.path.join(report_dir, filename))

    if not requested.startswith(report_dir + os.sep):
        abort(403)  # Path traversal attempt blocked

    if not os.path.isfile(requested):
        abort(404)

    return send_file(requested, as_attachment=True)


@app.route("/api/v1/datasets/checksum", methods=["POST"])
def compute_checksum():
    """
    Compute a SHA-256 checksum of uploaded data for integrity verification.
    Uses SHA-256 (strong hash) — not for passwords, just data integrity.
    """
    data = request.data
    digest = hashlib.sha256(data).hexdigest()
    return jsonify({"sha256": digest, "size": len(data)})


# ---------------------------------------------------------------------------
# Startup
# ---------------------------------------------------------------------------
if __name__ == "__main__":
    init_db()
    app.run(host="0.0.0.0", port=5000, debug=True)
