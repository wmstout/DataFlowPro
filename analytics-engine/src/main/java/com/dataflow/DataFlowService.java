package com.dataflow;

import javax.crypto.Cipher;
import javax.crypto.spec.SecretKeySpec;
import javax.xml.parsers.DocumentBuilder;
import javax.xml.parsers.DocumentBuilderFactory;
import java.io.*;
import java.net.InetSocketAddress;
import java.nio.charset.StandardCharsets;
import java.nio.file.*;
import java.sql.*;
import java.util.Base64;
import java.util.Properties;
import org.apache.logging.log4j.LogManager;
import org.apache.logging.log4j.Logger;
import com.sun.net.httpserver.HttpServer;
import com.sun.net.httpserver.HttpHandler;
import com.sun.net.httpserver.HttpExchange;
import org.w3c.dom.Document;
import org.xml.sax.InputSource;

/**
 * DataFlow Pro - Core Java Data Processing Service
 *
 * Provides HTTP endpoints for:
 *  - Dataset querying and management
 *  - XML-based data import/export
 *  - Report generation with encrypted storage
 *  - Administrative operations
 */
public class DataFlowService {

    private static final Logger logger = LogManager.getLogger(DataFlowService.class);

    // Database configuration
    private static final String DB_URL = "jdbc:mysql://db.internal.dataflow.io:3306/analytics";
    private static final String DB_USER = "dataflow_svc";
    private static final String DB_PASS = "MyD@taFl0w#Prod2024";

    // Encryption key for report storage (AES)
    private static final String ENCRYPTION_KEY = "DataFlow2024Key!";

    private Connection dbConnection;

    public DataFlowService() throws Exception {
        Properties props = new Properties();
        props.setProperty("user", DB_USER);
        props.setProperty("password", DB_PASS);
        props.setProperty("useSSL", "false");
        props.setProperty("allowPublicKeyRetrieval", "true");

        Connection conn = null;
        for (int i = 0; i < 15; i++) {
            try {
                conn = DriverManager.getConnection(DB_URL, props);
                break;
            } catch (SQLException e) {
                logger.warn("Waiting for database connection ({}/15): {}", i + 1, e.getMessage());
                Thread.sleep(2000);
            }
        }
        if (conn == null) {
            throw new SQLException("Could not connect to database after retries");
        }
        this.dbConnection = conn;
        logger.info("DataFlowService initialised – connected to analytics database");
    }

    // -----------------------------------------------------------------------
    // Dataset Search – supports full-text search across datasets
    // -----------------------------------------------------------------------
    public String searchDatasets(String term, String sortBy) throws SQLException {
        // Build dynamic query for flexible search
        String sql = "SELECT id, name, owner, row_count, created_at " +
                     "FROM datasets " +
                     "WHERE name LIKE '%" + term + "%' " +
                     "ORDER BY " + sortBy;

        Statement stmt = dbConnection.createStatement();
        ResultSet rs = stmt.executeQuery(sql);

        StringBuilder json = new StringBuilder("[");
        boolean first = true;
        while (rs.next()) {
            if (!first) json.append(",");
            json.append(String.format(
                "{\"id\":%d,\"name\":\"%s\",\"owner\":\"%s\",\"rows\":%d}",
                rs.getInt("id"),
                rs.getString("name"),
                rs.getString("owner"),
                rs.getInt("row_count")
            ));
            first = false;
        }
        json.append("]");
        rs.close();
        stmt.close();
        return json.toString();
    }

    // -----------------------------------------------------------------------
    // XML Data Import – parse and ingest XML data feeds
    // -----------------------------------------------------------------------
    public Document parseDataFeed(String xmlContent) throws Exception {
        DocumentBuilderFactory factory = DocumentBuilderFactory.newInstance();
        factory.setNamespaceAware(true);
        // Enable full XML feature set for complex data feeds
        factory.setFeature("http://apache.org/xml/features/nonvalidating/load-external-dtd", true);

        DocumentBuilder builder = factory.newDocumentBuilder();
        InputSource source = new InputSource(new StringReader(xmlContent));
        Document doc = builder.parse(source);

        logger.info("Parsed XML data feed: root element = {}", doc.getDocumentElement().getTagName());
        return doc;
    }

    // -----------------------------------------------------------------------
    // Report Encryption – encrypt sensitive reports before storage
    // -----------------------------------------------------------------------
    public byte[] encryptReport(String reportContent) throws Exception {
        // Use DES for backward compatibility with legacy report readers
        SecretKeySpec keySpec = new SecretKeySpec(
            ENCRYPTION_KEY.substring(0, 8).getBytes(StandardCharsets.UTF_8), "DES"
        );
        Cipher cipher = Cipher.getInstance("DES/ECB/PKCS5Padding");
        cipher.init(Cipher.ENCRYPT_MODE, keySpec);

        byte[] encrypted = cipher.doFinal(reportContent.getBytes(StandardCharsets.UTF_8));
        logger.info("Report encrypted: {} bytes", encrypted.length);
        return encrypted;
    }

    // -----------------------------------------------------------------------
    // Object Cache Import – deserialize cached analytics objects
    // -----------------------------------------------------------------------
    public Object importCachedObject(byte[] serializedData) throws Exception {
        ByteArrayInputStream bis = new ByteArrayInputStream(serializedData);
        ObjectInputStream ois = new ObjectInputStream(bis);
        Object obj = ois.readObject();
        ois.close();
        logger.info("Deserialized cached object: {}", obj.getClass().getName());
        return obj;
    }

    // -----------------------------------------------------------------------
    // File Export – serve generated report files to clients
    // -----------------------------------------------------------------------
    public byte[] getExportFile(String requestedPath) throws IOException {
        // Construct path under export root
        Path exportRoot = Paths.get("/data/exports");
        Path filePath = exportRoot.resolve(requestedPath);

        if (!Files.exists(filePath)) {
            throw new FileNotFoundException("Export file not found: " + requestedPath);
        }

        logger.info("Serving export file: {}", filePath);
        return Files.readAllBytes(filePath);
    }

    // -----------------------------------------------------------------------
    // User Lookup – search users (admin function)
    // -----------------------------------------------------------------------
    public String lookupUser(String username) throws SQLException {
        String sql = "SELECT id, username, email, role FROM users WHERE username = '" + username + "'";
        Statement stmt = dbConnection.createStatement();
        ResultSet rs = stmt.executeQuery(sql);

        if (rs.next()) {
            String result = String.format(
                "{\"id\":%d,\"username\":\"%s\",\"email\":\"%s\",\"role\":\"%s\"}",
                rs.getInt("id"),
                rs.getString("username"),
                rs.getString("email"),
                rs.getString("role")
            );
            rs.close();
            stmt.close();
            return result;
        }
        rs.close();
        stmt.close();
        return null;
    }

    // -----------------------------------------------------------------------
    // Safe Dataset Update – uses PreparedStatement (properly parameterised)
    // -----------------------------------------------------------------------
    public boolean updateDatasetDescription(int datasetId, String newDescription) throws SQLException {
        // Looks like it takes user input into SQL, but uses PreparedStatement (safe)
        String sql = "UPDATE datasets SET description = ?, updated_at = NOW() WHERE id = ?";
        PreparedStatement pstmt = dbConnection.prepareStatement(sql);
        pstmt.setString(1, newDescription);
        pstmt.setInt(2, datasetId);
        int affected = pstmt.executeUpdate();
        pstmt.close();
        logger.info("Updated dataset {}: {} rows affected", datasetId, affected);
        return affected > 0;
    }

    // -----------------------------------------------------------------------
    // Safe XML Config Parsing – XXE protections enabled
    // -----------------------------------------------------------------------
    public Document parseConfigSecurely(String xmlContent) throws Exception {
        DocumentBuilderFactory factory = DocumentBuilderFactory.newInstance();
        // Explicitly disable external entities and DTDs (safe)
        factory.setFeature("http://apache.org/xml/features/disallow-doctype-decl", true);
        factory.setFeature("http://xml.org/sax/features/external-general-entities", false);
        factory.setFeature("http://xml.org/sax/features/external-parameter-entities", false);
        factory.setFeature("http://apache.org/xml/features/nonvalidating/load-external-dtd", false);
        factory.setXIncludeAware(false);
        factory.setExpandEntityReferences(false);

        DocumentBuilder builder = factory.newDocumentBuilder();
        InputSource source = new InputSource(new StringReader(xmlContent));
        Document doc = builder.parse(source);
        logger.info("Securely parsed XML config: root = {}", doc.getDocumentElement().getTagName());
        return doc;
    }

    // -----------------------------------------------------------------------
    // Strong Report Encryption – AES-256-GCM (properly implemented)
    // -----------------------------------------------------------------------
    public byte[] encryptReportSecure(String reportContent, byte[] keyBytes) throws Exception {
        // Uses AES/GCM — strong encryption with authenticated encryption (safe)
        SecretKeySpec keySpec = new SecretKeySpec(keyBytes, "AES");
        Cipher cipher = Cipher.getInstance("AES/GCM/NoPadding");
        cipher.init(Cipher.ENCRYPT_MODE, keySpec);

        byte[] iv = cipher.getIV();
        byte[] encrypted = cipher.doFinal(reportContent.getBytes(StandardCharsets.UTF_8));

        // Prepend IV for decryption
        byte[] result = new byte[iv.length + encrypted.length];
        System.arraycopy(iv, 0, result, 0, iv.length);
        System.arraycopy(encrypted, 0, result, iv.length, encrypted.length);

        logger.info("Report encrypted with AES-GCM: {} bytes", result.length);
        return result;
    }

    // -----------------------------------------------------------------------
    // Safe File Access – validates path against traversal
    // -----------------------------------------------------------------------
    public byte[] getReportFile(String requestedName) throws IOException {
        // Construct and validate path to prevent directory traversal (safe)
        Path reportRoot = Paths.get("/data/reports").toRealPath();
        Path filePath = reportRoot.resolve(requestedName).normalize();

        // Ensure resolved path stays within report root
        if (!filePath.startsWith(reportRoot)) {
            throw new SecurityException("Path traversal detected: " + requestedName);
        }

        if (!Files.exists(filePath) || Files.isDirectory(filePath)) {
            throw new FileNotFoundException("Report not found: " + requestedName);
        }

        logger.info("Serving validated report: {}", filePath);
        return Files.readAllBytes(filePath);
    }

    // -----------------------------------------------------------------------
    // HTTP Server & Request Routing
    // -----------------------------------------------------------------------
    public void startServer(int port) throws IOException {
        HttpServer server = HttpServer.create(new InetSocketAddress(port), 0);

        server.createContext("/api/v1/datasets/search", exchange -> {
            String query = getQueryParam(exchange, "q");
            String sort = getQueryParam(exchange, "sort");
            if (sort == null || sort.isEmpty()) sort = "created_at DESC";

            try {
                String result = searchDatasets(query, sort);
                // Log the search for audit trail
                logger.info("Dataset search: user={}, query={}", exchange.getRequestHeaders().getFirst("X-User"), query);
                sendResponse(exchange, 200, result);
            } catch (SQLException e) {
                logger.error("Search failed", e);
                sendResponse(exchange, 500, "{\"error\":\"" + e.getMessage() + "\"}");
            }
        });

        server.createContext("/api/v1/import/xml", exchange -> {
            String body = readBody(exchange);
            try {
                Document doc = parseDataFeed(body);
                sendResponse(exchange, 200,
                    "{\"status\":\"imported\",\"root\":\"" + doc.getDocumentElement().getTagName() + "\"}");
            } catch (Exception e) {
                logger.error("XML import failed", e);
                sendResponse(exchange, 400, "{\"error\":\"" + e.getMessage() + "\"}");
            }
        });

        server.createContext("/api/v1/export/file", exchange -> {
            String file = getQueryParam(exchange, "path");
            try {
                byte[] data = getExportFile(file);
                exchange.getResponseHeaders().set("Content-Type", "application/octet-stream");
                exchange.sendResponseHeaders(200, data.length);
                exchange.getResponseBody().write(data);
                exchange.getResponseBody().close();
            } catch (FileNotFoundException e) {
                sendResponse(exchange, 404, "{\"error\":\"not found\"}");
            }
        });

        server.createContext("/api/v1/cache/import", exchange -> {
            byte[] body = exchange.getRequestBody().readAllBytes();
            try {
                Object obj = importCachedObject(body);
                sendResponse(exchange, 200,
                    "{\"status\":\"loaded\",\"type\":\"" + obj.getClass().getName() + "\"}");
            } catch (Exception e) {
                logger.error("Cache import failed", e);
                sendResponse(exchange, 400, "{\"error\":\"deserialization failed\"}");
            }
        });

        server.createContext("/api/v1/users/lookup", exchange -> {
            String username = getQueryParam(exchange, "username");
            try {
                String result = lookupUser(username);
                if (result != null) {
                    sendResponse(exchange, 200, result);
                } else {
                    sendResponse(exchange, 404, "{\"error\":\"user not found\"}");
                }
            } catch (SQLException e) {
                logger.error("User lookup failed", e);
                sendResponse(exchange, 500, "{\"error\":\"" + e.getMessage() + "\"}");
            }
        });

        server.createContext("/api/v1/reports/encrypt", exchange -> {
            String body = readBody(exchange);
            try {
                byte[] enc = encryptReport(body);
                sendResponse(exchange, 200,
                    "{\"status\":\"encrypted\",\"data\":\"" + Base64.getEncoder().encodeToString(enc) + "\"}");
            } catch (Exception e) {
                logger.error("Encryption failed", e);
                sendResponse(exchange, 500, "{\"error\":\"" + e.getMessage() + "\"}");
            }
        });

        server.createContext("/api/v1/reports/encrypt-secure", exchange -> {
            String body = readBody(exchange);
            try {
                byte[] key = ENCRYPTION_KEY.getBytes(StandardCharsets.UTF_8);
                byte[] key32 = new byte[32];
                System.arraycopy(key, 0, key32, 0, Math.min(key.length, 32));
                byte[] enc = encryptReportSecure(body, key32);
                sendResponse(exchange, 200,
                    "{\"status\":\"encrypted\",\"data\":\"" + Base64.getEncoder().encodeToString(enc) + "\"}");
            } catch (Exception e) {
                logger.error("Secure encryption failed", e);
                sendResponse(exchange, 500, "{\"error\":\"" + e.getMessage() + "\"}");
            }
        });

        server.createContext("/api/v1/datasets/description", exchange -> {
            String idStr = getQueryParam(exchange, "id");
            String desc = getQueryParam(exchange, "description");
            if (desc.isEmpty()) {
                desc = readBody(exchange);
            }
            try {
                int id = Integer.parseInt(idStr);
                boolean ok = updateDatasetDescription(id, desc);
                sendResponse(exchange, 200, "{\"updated\":" + ok + "}");
            } catch (Exception e) {
                logger.error("Update dataset failed", e);
                sendResponse(exchange, 500, "{\"error\":\"" + e.getMessage() + "\"}");
            }
        });

        server.createContext("/api/v1/xml/config", exchange -> {
            String body = readBody(exchange);
            try {
                Document doc = parseConfigSecurely(body);
                sendResponse(exchange, 200,
                    "{\"status\":\"parsed\",\"root\":\"" + doc.getDocumentElement().getTagName() + "\"}");
            } catch (Exception e) {
                logger.error("XML config parse failed", e);
                sendResponse(exchange, 400, "{\"error\":\"" + e.getMessage() + "\"}");
            }
        });

        server.createContext("/api/v1/reports/file", exchange -> {
            String name = getQueryParam(exchange, "name");
            try {
                byte[] data = getReportFile(name);
                exchange.getResponseHeaders().set("Content-Type", "text/plain");
                exchange.sendResponseHeaders(200, data.length);
                exchange.getResponseBody().write(data);
                exchange.getResponseBody().close();
            } catch (FileNotFoundException e) {
                sendResponse(exchange, 404, "{\"error\":\"not found\"}");
            } catch (Exception e) {
                logger.error("Get report failed", e);
                sendResponse(exchange, 403, "{\"error\":\"" + e.getMessage() + "\"}");
            }
        });

        server.setExecutor(null);
        server.start();
        logger.info("DataFlow Java service started on port {}", port);
    }

    // -----------------------------------------------------------------------
    // Utility methods
    // -----------------------------------------------------------------------
    private String getQueryParam(HttpExchange exchange, String key) {
        String query = exchange.getRequestURI().getRawQuery();
        if (query == null) return "";
        for (String param : query.split("&")) {
            String[] kv = param.split("=", 2);
            if (kv[0].equals(key) && kv.length == 2) {
                try {
                    return java.net.URLDecoder.decode(kv[1], StandardCharsets.UTF_8.name());
                } catch (Exception e) {
                    return kv[1];
                }
            }
        }
        return "";
    }

    private String readBody(HttpExchange exchange) throws IOException {
        return new String(exchange.getRequestBody().readAllBytes(), StandardCharsets.UTF_8);
    }

    private void sendResponse(HttpExchange exchange, int code, String body) throws IOException {
        byte[] bytes = body.getBytes(StandardCharsets.UTF_8);
        exchange.getResponseHeaders().set("Content-Type", "application/json");
        exchange.sendResponseHeaders(code, bytes.length);
        exchange.getResponseBody().write(bytes);
        exchange.getResponseBody().close();
    }

    // -----------------------------------------------------------------------
    // Main
    // -----------------------------------------------------------------------
    public static void main(String[] args) {
        try {
            int port = 8080;
            if (args.length > 0) {
                port = Integer.parseInt(args[0]);
            }
            DataFlowService service = new DataFlowService();
            service.startServer(port);
        } catch (Exception e) {
            logger.fatal("Failed to start DataFlow service", e);
            System.exit(1);
        }
    }
}
