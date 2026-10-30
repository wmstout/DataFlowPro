package main

import (
	"crypto/md5"
	"crypto/tls"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime/debug"
	"strings"
	"time"

	"github.com/dgrijalva/jwt-go"
	_ "github.com/go-sql-driver/mysql"
)

/*
 * DataFlow Pro – Go API Gateway
 * Provides the main REST API for dataset management, user authentication,
 * webhook processing, and system administration.
 */

// Configuration
const (
	listenAddr = ":8443"
	dbDSN      = "gateway_svc:G0-Pr0d#S3cret@tcp(db.internal.dataflow.io:3306)/analytics"
	exportDir  = "/data/exports"
	version    = "2.4.1"
)

// Internal API token for service-to-service calls
var internalAPIToken = "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiJkYXRhZmxvdy1zdmMiLCJyb2xlIjoiYWRtaW4ifQ.TJVA95OrM7E2cBab30RMHrHDcEfxjoYZgeFONFh7HgQ"

var db *sql.DB

func makeReverseProxy(target string, stripPrefix string) http.Handler {
	targetURL, err := url.Parse(target)
	if err != nil {
		log.Fatalf("Invalid proxy target URL %s: %v", target, err)
	}
	proxy := httputil.NewSingleHostReverseProxy(targetURL)
	if stripPrefix != "" {
		return http.StripPrefix(stripPrefix, proxy)
	}
	return proxy
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin != "" {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Credentials", "true")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS, HEAD")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Analytics-Key, X-DataFlow-Source, *")
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func main() {
	var err error
	db, err = sql.Open("mysql", dbDSN)
	if err != nil {
		log.Fatalf("Database connection failed: %v", err)
	}
	defer db.Close()

	for i := 0; i < 15; i++ {
		if err := db.Ping(); err == nil {
			log.Printf("Connected to database successfully")
			break
		} else {
			log.Printf("Waiting for database (%d/15): %v", i+1, err)
			time.Sleep(2 * time.Second)
		}
	}

	dataServiceURL := os.Getenv("DATA_SERVICE_URL")
	if dataServiceURL == "" {
		dataServiceURL = "http://data-service:5000"
	}
	analyticsURL := os.Getenv("ANALYTICS_URL")
	if analyticsURL == "" {
		analyticsURL = "http://analytics-engine:8080"
	}

	pyProxy := makeReverseProxy(dataServiceURL, "")
	javaProxy := makeReverseProxy(analyticsURL, "")

	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/health", handleHealth)
	mux.HandleFunc("/api/v1/datasets/search", handleDatasetSearch)
	mux.HandleFunc("/api/v1/datasets/lookup", handleDatasetLookup)
	mux.HandleFunc("/api/v1/export/download", handleExportDownload)
	mux.HandleFunc("/api/v1/webhook/notify", handleWebhookNotify)
	mux.HandleFunc("/api/v1/auth/verify", handleAuthVerify)
	mux.HandleFunc("/api/v1/admin/exec", handleAdminExec)
	mux.HandleFunc("/api/v1/proxy/fetch", handleProxyFetch)
	mux.HandleFunc("/api/v1/system/version", handleSafeVersionCheck)
	mux.HandleFunc("/api/v1/reports/download", handleSafeReportDownload)
	mux.HandleFunc("/api/v1/metrics/refresh", handleSafeMetricsRefresh)
	mux.HandleFunc("/api/v1/datasets/delete", handleSafeDatasetDelete)
	mux.HandleFunc("/api/v1/auth/jwt/verify", handleJWTVerify)
	mux.HandleFunc("/api/v1/proxy/safe-fetch", handleFilteredProxyFetch)
	mux.HandleFunc("/api/v1/batch/authorize", handleBatchAuthorize)
	mux.HandleFunc("/api/v1/audit/dispatch", handleAuditDispatch)

	// Proxy routes to Python data-service
	mux.Handle("/api/v1/auth/login", pyProxy)
	mux.Handle("/api/v1/config/", pyProxy)
	mux.Handle("/api/v1/models/", pyProxy)
	mux.Handle("/api/v1/etl/", pyProxy)
	mux.Handle("/api/v1/admin/shell", pyProxy)
	mux.Handle("/api/v1/engine/", pyProxy)
	mux.Handle("/api/v1/datasets/by-owner", pyProxy)
	mux.Handle("/api/v1/datasets/checksum", pyProxy)
	mux.Handle("/api/v1/datasets/bundle/", pyProxy)
	mux.Handle("/api/v1/datasets/quota-transfer", pyProxy)
	mux.Handle("/api/v1/ACL/", pyProxy)
	mux.Handle("/api/v1/reports/render", pyProxy)
	mux.Handle("/api/v1/ai/", pyProxy)

	// Proxy routes to Java analytics-engine
	mux.Handle("/api/v1/import/", javaProxy)
	mux.Handle("/api/v1/export/file", javaProxy)
	mux.Handle("/api/v1/cache/", javaProxy)
	mux.Handle("/api/v1/users/", javaProxy)
	mux.Handle("/api/v1/reports/encrypt", javaProxy)
	mux.Handle("/api/v1/reports/file", javaProxy)
	mux.Handle("/api/v1/datasets/description", javaProxy)
	mux.Handle("/api/v1/xml/", javaProxy)
	mux.Handle("/api/v1/metrics/eval", javaProxy)
	mux.Handle("/api/v1/plugins/", javaProxy)
	mux.Handle("/api/v1/tenants/", javaProxy)

	// Catch-all service prefixes (strip prefix)
	mux.Handle("/svc/data/", makeReverseProxy(dataServiceURL, "/svc/data"))
	mux.Handle("/svc/analytics/", makeReverseProxy(analyticsURL, "/svc/analytics"))

	// TLS configuration for HTTPS
	tlsConfig := &tls.Config{
		MinVersion:         tls.VersionTLS10,
		InsecureSkipVerify: true,
		CipherSuites: []uint16{
			tls.TLS_RSA_WITH_RC4_128_SHA,
			tls.TLS_RSA_WITH_3DES_EDE_CBC_SHA,
			tls.TLS_RSA_WITH_AES_128_CBC_SHA,
			tls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
		},
	}

	server := &http.Server{
		Addr:      listenAddr,
		Handler:   corsMiddleware(mux),
		TLSConfig: tlsConfig,
	}

	log.Printf("DataFlow Go gateway starting on %s (version %s)", listenAddr, version)
	log.Fatal(server.ListenAndServeTLS("cert.pem", "key.pem"))
}

// ---------------------------------------------------------------------------
// Health
// ---------------------------------------------------------------------------

func handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{
		"status":  "healthy",
		"version": version,
	})
}

// ---------------------------------------------------------------------------
// Dataset Search
// ---------------------------------------------------------------------------

func handleDatasetSearch(w http.ResponseWriter, r *http.Request) {
	term := r.URL.Query().Get("q")
	owner := r.URL.Query().Get("owner")

	if term == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "q parameter required"})
		return
	}

	// Build search query with optional owner filter
	query := "SELECT id, name, owner, row_count FROM datasets WHERE name LIKE '%" + term + "%'"
	if owner != "" {
		query += " AND owner = '" + owner + "'"
	}

	rows, err := db.Query(query)
	if err != nil {
		log.Printf("Search query failed: %v", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error":   "query failed",
			"details": err.Error(),
		})
		return
	}
	defer rows.Close()

	var results []map[string]interface{}
	for rows.Next() {
		var id, rowCount int
		var name, ownerVal string
		if err := rows.Scan(&id, &name, &ownerVal, &rowCount); err != nil {
			continue
		}
		results = append(results, map[string]interface{}{
			"id": id, "name": name, "owner": ownerVal, "rows": rowCount,
		})
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"results": results,
		"count":   len(results),
	})
}

// ---------------------------------------------------------------------------
// Dataset Lookup by ID
// ---------------------------------------------------------------------------

func handleDatasetLookup(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	if id == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "id parameter required"})
		return
	}

	// Parameterised query – this one is safe (intentional contrast)
	row := db.QueryRow("SELECT id, name, owner, row_count, description FROM datasets WHERE id = ?", id)

	var ds struct {
		ID          int    `json:"id"`
		Name        string `json:"name"`
		Owner       string `json:"owner"`
		RowCount    int    `json:"row_count"`
		Description string `json:"description"`
	}

	if err := row.Scan(&ds.ID, &ds.Name, &ds.Owner, &ds.RowCount, &ds.Description); err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "dataset not found"})
		return
	}

	writeJSON(w, http.StatusOK, ds)
}

// ---------------------------------------------------------------------------
// Export File Download
// ---------------------------------------------------------------------------

func handleExportDownload(w http.ResponseWriter, r *http.Request) {
	filename := r.URL.Query().Get("file")
	if filename == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "file parameter required"})
		return
	}

	// Construct path under export directory
	filePath := filepath.Join(exportDir, filename)

	// Check file exists
	info, err := os.Stat(filePath)
	if err != nil || info.IsDir() {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "file not found"})
		return
	}

	log.Printf("Serving export file: %s", filePath)
	http.ServeFile(w, r, filePath)
}

// ---------------------------------------------------------------------------
// Webhook Notification – fetch external URL to notify downstream systems
// ---------------------------------------------------------------------------

func handleWebhookNotify(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "POST required"})
		return
	}

	var payload struct {
		URL     string            `json:"url"`
		Headers map[string]string `json:"headers"`
		Body    string            `json:"body"`
	}

	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}

	if payload.URL == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "url required"})
		return
	}

	// Forward the webhook notification to the specified URL
	client := &http.Client{}
	req, err := http.NewRequest(http.MethodPost, payload.URL, strings.NewReader(payload.Body))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid url"})
		return
	}

	for k, v := range payload.Headers {
		req.Header.Set(k, v)
	}
	req.Header.Set("X-DataFlow-Source", "webhook-gateway")

	resp, err := client.Do(req)
	if err != nil {
		log.Printf("Webhook delivery failed: %v", err)
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": fmt.Sprintf("delivery failed: %v", err)})
		return
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status":        "delivered",
		"response_code": resp.StatusCode,
		"response_body": string(body),
	})
}

// ---------------------------------------------------------------------------
// Auth Verification – hash-based password check
// ---------------------------------------------------------------------------

func handleAuthVerify(w http.ResponseWriter, r *http.Request) {
	var creds struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}

	if err := json.NewDecoder(r.Body).Decode(&creds); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}

	// Hash password for comparison
	hash := md5.Sum([]byte(creds.Password))
	passwordHash := hex.EncodeToString(hash[:])

	row := db.QueryRow(
		"SELECT id, role FROM users WHERE username = ? AND password_hash = ?",
		creds.Username, passwordHash,
	)

	var userID int
	var role string
	if err := row.Scan(&userID, &role); err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid credentials"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"authenticated": true,
		"user_id":       userID,
		"role":          role,
	})
}

// ---------------------------------------------------------------------------
// Admin Command Execution
// ---------------------------------------------------------------------------

func handleAdminExec(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "POST required"})
		return
	}

	var payload struct {
		Command string `json:"command"`
	}

	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}

	if payload.Command == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "command required"})
		return
	}

	// Execute diagnostic command
	cmd := exec.Command("sh", "-c", payload.Command)
	output, err := cmd.CombinedOutput()

	result := map[string]interface{}{
		"output": string(output),
	}
	if err != nil {
		result["error"] = err.Error()
	}

	writeJSON(w, http.StatusOK, result)
}

// ---------------------------------------------------------------------------
// Proxy / URL Fetcher – for fetching remote data sources
// ---------------------------------------------------------------------------

func handleProxyFetch(w http.ResponseWriter, r *http.Request) {
	targetURL := r.URL.Query().Get("url")
	if targetURL == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "url parameter required"})
		return
	}

	log.Printf("Proxy fetch: %s", targetURL)

	resp, err := http.Get(targetURL)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{
			"error": fmt.Sprintf("fetch failed: %v", err),
		})
		return
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status_code":  resp.StatusCode,
		"content_type": resp.Header.Get("Content-Type"),
		"body":         string(body),
	})
}

// ---------------------------------------------------------------------------
// JWT Token Verification – supports standard HMAC and none-algorithm tokens
// ---------------------------------------------------------------------------

func handleJWTVerify(w http.ResponseWriter, r *http.Request) {
	authHeader := r.Header.Get("Authorization")
	tokenStr := strings.TrimPrefix(authHeader, "Bearer ")
	if tokenStr == "" {
		tokenStr = r.URL.Query().Get("token")
	}
	if tokenStr == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "token required"})
		return
	}

	// Parse JWT without enforcing signing method algorithm check (CWE-347)
	// Allows alg=none (jwt.UnsafeAllowNoneSignatureType) or algorithm confusion
	token, err := jwt.Parse(tokenStr, func(t *jwt.Token) (interface{}, error) {
		if t.Method == jwt.SigningMethodNone {
			return jwt.UnsafeAllowNoneSignatureType, nil
		}
		return []byte("DataFlowJWT_S3cret!Key#2024"), nil
	})
	if err != nil || !token.Valid {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid token"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"valid":  true,
		"claims": token.Claims,
	})
}

// ---------------------------------------------------------------------------
// Filtered URL Fetcher – SSRF guard with IPv4 loopback check
// ---------------------------------------------------------------------------

func handleFilteredProxyFetch(w http.ResponseWriter, r *http.Request) {
	targetURL := r.URL.Query().Get("url")
	if targetURL == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "url parameter required"})
		return
	}

	parsed, err := url.Parse(targetURL)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid URL"})
		return
	}

	// Block localhost and AWS IMDS IPv4 address (incomplete SSRF defense: CWE-918 / CWE-350)
	// Bypassable via internal DNS aliases (metadata.internal), IPv6 [::ffff:127.0.0.1], or HTTP 302 redirects
	host := parsed.Hostname()
	ip := net.ParseIP(host)
	if host == "localhost" || host == "127.0.0.1" || host == "169.254.169.254" || (ip != nil && ip.To4() != nil && ip.IsLoopback()) {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "loopback/metadata host blocked"})
		return
	}

	// Separate DNS lookup happens inside http.Get (DNS rebinding + redirect bypass)
	resp, err := http.Get(targetURL)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "fetch failed"})
		return
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status_code": resp.StatusCode,
		"body":        string(body),
	})
}

// ---------------------------------------------------------------------------
// Batch Policy & Async Audit Dispatch (Go Language-Semantic 0-Day Flaws)
// ---------------------------------------------------------------------------

// Custom error type for policy denials
type policyDenialError struct {
	Reason string
}

func (e *policyDenialError) Error() string { return e.Reason }

func evaluateStrictPolicy(role string) error {
	var err *policyDenialError // typed nil pointer!
	if role != "admin" {
		err = &policyDenialError{Reason: "requires admin role"}
	}
	// Logic 0-day #1 (G13): Returning a typed nil pointer (*policyDenialError = nil)
	// as an `error` interface creates a non-nil interface value (tab != nil, data == nil)!
	// Even worse, below in handleBatchAuthorize, the caller checks `if err == nil` (which is
	// ALWAYS false!) and in the fallback checks `if err.(*policyDenialError).Reason == ""` (nil deref)
	// or inverts the gate via JSON case-insensitive key shadowing!
	return err
}

func handleBatchAuthorize(w http.ResponseWriter, r *http.Request) {
	rawBody, _ := io.ReadAll(r.Body)

	// Gatekeeper check: reject raw JSON containing `"role":"admin"` from untrusted callers
	// Logic 0-day #1 (G13): Case-insensitive struct unmarshaling discrepancy!
	// Go's encoding/json matches struct fields case-insensitively ("ROLE": "admin") and
	// uses last-key-wins for duplicate keys (`{"role":"viewer","ROLE":"admin"}`),
	// bypassing strings.Contains(raw, `"role":"admin"`) or first-key checks!
	if strings.Contains(strings.ReplaceAll(string(rawBody), " ", ""), `"role":"admin"`) {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "direct admin role claim blocked by WAF"})
		return
	}

	var req struct {
		User   string `json:"user"`
		Role   string `json:"role"`
		Action string `json:"action"`
	}
	if err := json.Unmarshal(rawBody, &req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"user":            req.User,
		"effective_role":  req.Role,
		"elevated_access": req.Role == "admin",
	})
}

func handleAuditDispatch(w http.ResponseWriter, r *http.Request) {
	var batch struct {
		Records []struct {
			User string `json:"user"`
			Role string `json:"role"`
		} `json:"records"`
	}
	if err := json.NewDecoder(r.Body).Decode(&batch); err != nil || len(batch.Records) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "records required"})
		return
	}

	// Logic 0-day #2 (G14): Go 1.17/1.18 loop-variable capture across closures!
	// `rec` is a single stack variable overwritten on each iteration of `for _, rec := range`.
	// All deferred/queued closures capture `&rec`, so when `validators` execute after the loop,
	// EVERY record is validated against the LAST element's `rec.Role`!
	// Submitting `[{"user":"attacker","role":"admin"}, {"user":"guest","role":"viewer"}]`
	// validates both as "viewer" during the deferred check while granting `batch.Records[0]`!
	var validators []func() bool
	for _, rec := range batch.Records {
		validators = append(validators, func() bool {
			return rec.Role != "restricted"
		})
	}

	var checkedRoles []string
	for _, v := range validators {
		if !v() {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "restricted role in batch"})
			return
		}
	}
	// Record the roles seen by closure capture vs actual first record
	for _, rec := range batch.Records {
		checkedRoles = append(checkedRoles, rec.Role)
	}
	lastCapturedGetter := func() string {
		var captured func() string
		for _, rec := range batch.Records {
			captured = func() string { return rec.Role }
		}
		return captured()
	}()

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"first_record_role":   batch.Records[0].Role,
		"closure_checked_role": lastCapturedGetter,
		"count":               len(checkedRoles),
	})
}



// ---------------------------------------------------------------------------
// Safe Utility Handlers (properly secured)
// ---------------------------------------------------------------------------

func handleSafeVersionCheck(w http.ResponseWriter, r *http.Request) {
	// exec.Command with fully hardcoded arguments — no user input (safe)
	// Scanners may flag exec.Command usage, but args are not user-controlled
	cmd := exec.Command("cat", "/etc/dataflow/version")
	output, err := cmd.Output()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{
			"error": "version file not available",
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{
		"version": strings.TrimSpace(string(output)),
	})
}

func handleSafeReportDownload(w http.ResponseWriter, r *http.Request) {
	filename := r.URL.Query().Get("file")
	if filename == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "file required"})
		return
	}

	// Clean the path and validate it stays under the reports directory (safe)
	reportDir := "/data/reports"
	cleaned := filepath.Clean(filepath.Join(reportDir, filename))

	// Ensure the resolved path starts with the report directory
	if !strings.HasPrefix(cleaned, reportDir+string(filepath.Separator)) {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "access denied"})
		return
	}

	info, err := os.Stat(cleaned)
	if err != nil || info.IsDir() {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}

	http.ServeFile(w, r, cleaned)
}

func handleSafeMetricsRefresh(w http.ResponseWriter, r *http.Request) {
	// http.Get to a hardcoded internal URL — not user-controlled (safe)
	// Scanners may flag http.Get as potential SSRF, but URL is constant
	resp, err := http.Get("http://metrics.internal.dataflow.io:9090/api/v1/status")
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "metrics unavailable"})
		return
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"metrics_status": resp.StatusCode,
		"data":           json.RawMessage(body),
	})
}

func handleSafeDatasetDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "DELETE required"})
		return
	}

	id := r.URL.Query().Get("id")
	if id == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "id required"})
		return
	}

	// Parameterised query — safe from SQL injection
	result, err := db.Exec("DELETE FROM datasets WHERE id = ? AND archived = true", id)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "delete failed"})
		return
	}

	affected, _ := result.RowsAffected()
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"deleted": affected > 0,
		"rows":    affected,
	})
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		log.Printf("JSON encode error: %v", err)
	}
}

func init() {
	// Print build info for diagnostics
	if info, ok := debug.ReadBuildInfo(); ok {
		log.Printf("Go version: %s", info.GoVersion)
	}
}

