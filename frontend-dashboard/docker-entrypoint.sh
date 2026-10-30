#!/bin/sh
set -e
API_BASE="${API_BASE:-https://localhost:8443/api/v1}"
echo "window.DATAFLOW_API_BASE = '${API_BASE}';" > /app/src/config.js
exec "$@"
