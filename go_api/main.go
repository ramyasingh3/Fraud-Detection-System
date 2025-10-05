package main

import (
    "context"
    "database/sql"
    "encoding/json"
    "fmt"
    "log"
    "net/http"
    "os"
    "strconv"
    "strings"
    "time"

    _ "github.com/lib/pq"
    "github.com/segmentio/kafka-go"
    "github.com/go-redis/redis/v8"
    "google.golang.org/grpc"
    "google.golang.org/grpc/credentials/insecure"

    pb "example.com/fraud/go_api/internal/pb"
)

type TransactionRequest struct {
    UserID         string   `json:"user_id"`
    Amount         float64  `json:"amount"`
    MerchantID     string   `json:"merchant_id"`
    MerchantRisk   float64  `json:"merchant_risk"`
    LocationLat    *float64 `json:"location_lat,omitempty"`
    LocationLon    *float64 `json:"location_lon,omitempty"`
    DeviceID       *string  `json:"device_id,omitempty"`
    IPAddress      *string  `json:"ip_address,omitempty"`
}

type TransactionResponse struct {
    TransactionID    string   `json:"transaction_id"`
    IsFraud          bool     `json:"is_fraud"`
    FraudScore       float64  `json:"fraud_score"`
    Confidence       float64  `json:"confidence"`
    RiskFactors      []string `json:"risk_factors"`
    ProcessingTimeMs int      `json:"processing_time_ms"`
}

type BatchTransactionRequest struct {
    Transactions []TransactionRequest `json:"transactions"`
}

type BatchTransactionResponse struct {
    Results               []TransactionResponse `json:"results"`
    TotalProcessingTimeMs int                   `json:"total_processing_time_ms"`
}

var (
    pg       *sql.DB
    rdb      *redis.Client
    kafkaW   *kafka.Writer
    ctx      = context.Background()
)

func getenv(key, def string) string {
    if v := os.Getenv(key); v != "" {
        return v
    }
    return def
}

func initConnections() error {
    // Postgres
    pgHost := getenv("POSTGRES_HOST", "localhost")
    pgDB := getenv("POSTGRES_DB", "fraud_detection")
    pgUser := getenv("POSTGRES_USER", "fraud_user")
    pgPass := getenv("POSTGRES_PASSWORD", "fraud_password")
    dsn := fmt.Sprintf("host=%s dbname=%s user=%s password=%s sslmode=disable", pgHost, pgDB, pgUser, pgPass)
    var err error
    pg, err = sql.Open("postgres", dsn)
    if err != nil { return err }
    if err = pg.Ping(); err != nil { return err }

    // Redis
    redisHost := getenv("REDIS_HOST", "localhost")
    redisPort := getenv("REDIS_PORT", "6379")
    rdb = redis.NewClient(&redis.Options{ Addr: redisHost+":"+redisPort })
    if err := rdb.Ping(ctx).Err(); err != nil { return err }

    // Kafka (best-effort)
    brokers := strings.Split(getenv("KAFKA_BOOTSTRAP_SERVERS", "localhost:9092"), ",")
    kafkaW = &kafka.Writer{Addr: kafka.TCP(brokers...), Topic: "fraud-transactions", Balancer: &kafka.LeastBytes{}}
    return nil
}

func rootHandler(w http.ResponseWriter, r *http.Request) {
    writeJSON(w, http.StatusOK, map[string]interface{}{"message": "Fraud Detection API (Go)", "status": "running"})
}

func healthHandler(w http.ResponseWriter, r *http.Request) {
    status := map[string]string{"redis": "down", "postgres": "down"}
    if err := rdb.Ping(ctx).Err(); err == nil { status["redis"] = "up" }
    if err := pg.Ping(); err == nil { status["postgres"] = "up" }
    writeJSON(w, http.StatusOK, map[string]interface{}{"status": "healthy", "services": status})
}

func processTransactionHandler(w http.ResponseWriter, r *http.Request) {
    start := time.Now()
    var req TransactionRequest
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        http.Error(w, err.Error(), http.StatusBadRequest)
        return
    }

    txID := fmt.Sprintf("%d", time.Now().UnixNano())
    cacheKey := "transaction:" + txID
    if cached, err := rdb.Get(ctx, cacheKey).Result(); err == nil {
        w.Header().Set("Content-Type", "application/json")
        w.Write([]byte(cached))
        return
    }

    // Feature engineering equivalents
    userRisk := getUserRiskScore(req.UserID)
    ratio := getAmountToHistoryRatio(req.UserID, req.Amount)

    // Scoring: optional gRPC to Python ML service if enabled, else placeholder
    useGRPC := strings.ToLower(getenv("USE_ML_GRPC", "false")) == "true"
    var (
        fraudScore float64
        confidence float64
        riskFactors []string
    )
    if useGRPC {
        // Attempt gRPC call; on error fallback to placeholder
        if fs, conf, rfs, err := getFraudScoreGRPC(req, userRisk, ratio); err == nil {
            fraudScore, confidence, riskFactors = fs, conf, rfs
        } else {
            fraudScore, confidence, riskFactors = getFraudScorePlaceholder(req.Amount, req.MerchantRisk, userRisk, ratio)
        }
    } else {
        fraudScore, confidence, riskFactors = getFraudScorePlaceholder(req.Amount, req.MerchantRisk, userRisk, ratio)
    }
    isFraud := fraudScore > 0.7

    // Ensure user exists (FK constraint)
    if err := ensureUserExists(req.UserID); err != nil {
        http.Error(w, "Failed to prepare user", http.StatusInternalServerError)
        return
    }

    // Store transaction
    if err := storeTransaction(txID, req, fraudScore, isFraud); err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }

    // Send to Kafka (best-effort)
    sendToKafka(txID, req, fraudScore, isFraud)

    resp := TransactionResponse{
        TransactionID:    txID,
        IsFraud:          isFraud,
        FraudScore:       fraudScore,
        Confidence:       confidence,
        RiskFactors:      riskFactors,
        ProcessingTimeMs: int(time.Since(start).Milliseconds()),
    }
    b, _ := json.Marshal(resp)
    _ = rdb.Set(ctx, cacheKey, string(b), 5*time.Minute).Err()
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(http.StatusOK)
    w.Write(b)
}

func batchProcessHandler(w http.ResponseWriter, r *http.Request) {
    start := time.Now()
    var req BatchTransactionRequest
    if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
        http.Error(w, err.Error(), http.StatusBadRequest)
        return
    }
    results := make([]TransactionResponse, 0, len(req.Transactions))
    for range req.Transactions {
        // Minimal stub: create placeholder responses
        results = append(results, TransactionResponse{TransactionID: fmt.Sprintf("%d", time.Now().UnixNano()), IsFraud: false, FraudScore: 0.5, Confidence: 0.8, RiskFactors: []string{"batch_processing"}, ProcessingTimeMs: 0})
    }
    writeJSON(w, http.StatusOK, BatchTransactionResponse{Results: results, TotalProcessingTimeMs: int(time.Since(start).Milliseconds())})
}

func getTransactionHandler(w http.ResponseWriter, r *http.Request) {
    // /transactions/{id}
    parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/transactions/"), "/")
    if len(parts) == 0 || parts[0] == "" {
        http.Error(w, "missing id", http.StatusBadRequest)
        return
    }
    id := parts[0]
    row := pg.QueryRow(`SELECT transaction_id, user_id, amount, timestamp, merchant_id, merchant_risk, fraud_score, is_fraud FROM transactions WHERE transaction_id = $1`, id)
    var (
        transactionID, userID, merchantID string
        amount, merchantRisk, fraudScore float64
        isFraud bool
        ts time.Time
    )
    if err := row.Scan(&transactionID, &userID, &amount, &ts, &merchantID, &merchantRisk, &fraudScore, &isFraud); err != nil {
        http.Error(w, "Transaction not found", http.StatusNotFound)
        return
    }
    writeJSON(w, http.StatusOK, map[string]interface{}{
        "transaction_id": transactionID,
        "user_id": userID,
        "amount": amount,
        "timestamp": ts,
        "merchant_id": merchantID,
        "merchant_risk": merchantRisk,
        "fraud_score": fraudScore,
        "is_fraud": isFraud,
    })
}

func userRiskHandler(w http.ResponseWriter, r *http.Request) {
    // /users/{id}/risk-score
    id := strings.TrimPrefix(r.URL.Path, "/users/")
    id = strings.TrimSuffix(id, "/risk-score")
    row := pg.QueryRow(`SELECT risk_score FROM users WHERE user_id = $1`, id)
    var risk float64
    if err := row.Scan(&risk); err != nil {
        http.Error(w, "User not found", http.StatusNotFound)
        return
    }
    writeJSON(w, http.StatusOK, map[string]interface{}{"user_id": id, "risk_score": risk})
}

func alertsHandler(w http.ResponseWriter, r *http.Request) {
    // /alerts?status=OPEN&limit=100
    q := r.URL.Query()
    status := q.Get("status")
    if status == "" { status = "OPEN" }
    limit := 100
    if s := q.Get("limit"); s != "" {
        if v, err := strconv.Atoi(s); err == nil { limit = v }
    }
    rows, err := pg.Query(`SELECT alert_id, transaction_id, alert_type, severity, description, confidence_score, status, created_at FROM fraud_alerts WHERE status = $1 ORDER BY created_at DESC LIMIT $2`, status, limit)
    if err != nil { http.Error(w, err.Error(), http.StatusInternalServerError); return }
    defer rows.Close()
    type Alert struct {
        AlertID        string      `json:"alert_id"`
        TransactionID  string      `json:"transaction_id"`
        AlertType      string      `json:"alert_type"`
        Severity       string      `json:"severity"`
        Description    string      `json:"description"`
        Confidence     float64     `json:"confidence_score"`
        Status         string      `json:"status"`
        CreatedAt      time.Time   `json:"created_at"`
    }
    var out []Alert
    for rows.Next() {
        var a Alert
        if err := rows.Scan(&a.AlertID, &a.TransactionID, &a.AlertType, &a.Severity, &a.Description, &a.Confidence, &a.Status, &a.CreatedAt); err != nil { continue }
        out = append(out, a)
    }
    writeJSON(w, http.StatusOK, out)
}

func getUserRiskScore(userID string) float64 {
    var risk float64 = 0.5
    row := pg.QueryRow(`SELECT risk_score FROM users WHERE user_id = $1`, userID)
    _ = row.Scan(&risk)
    return risk
}

func getAmountToHistoryRatio(userID string, amount float64) float64 {
    var avg sql.NullFloat64
    row := pg.QueryRow(`SELECT AVG(amount) FROM transactions WHERE user_id = $1`, userID)
    _ = row.Scan(&avg)
    base := 100.0
    if avg.Valid && avg.Float64 > 0 { base = avg.Float64 }
    return amount / base
}

func ensureUserExists(userID string) error {
    // Insert user with default risk score if not exists
    _, err := pg.Exec(`INSERT INTO users (user_id, risk_score) VALUES ($1, $2)
                       ON CONFLICT (user_id) DO NOTHING`, userID, 0.5)
    return err
}

func getFraudScorePlaceholder(amount, merchantRisk, userRisk, ratio float64) (float64, float64, []string) {
    score := 0.3
    if amount > 5000 { score += 0.3 }
    score += 0.2 * merchantRisk
    score += 0.1 * userRisk
    if ratio > 5 { score += 0.2 }
    if score > 1 { score = 1 }
    rf := []string{}
    if amount > 5000 { rf = append(rf, "high_amount") }
    if merchantRisk > 0.8 { rf = append(rf, "high_merchant_risk") }
    if userRisk > 0.7 { rf = append(rf, "high_user_risk") }
    if ratio > 5 { rf = append(rf, "unusual_amount_pattern") }
    return score, 0.8, rf
}

// getFraudScoreGRPC is a stub for calling the Python ML gRPC service.
// Replace with generated client from protos in /protos when available.
func getFraudScoreGRPC(req TransactionRequest, userRisk, ratio float64) (float64, float64, []string, error) {
    addr := getenv("ML_GRPC_ADDR", "fraud_ml:50051")
    conn, err := grpc.Dial(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
    if err != nil { return 0, 0, nil, err }
    defer conn.Close()

    client := pb.NewFraudDetectionServiceClient(conn)

    now := time.Now().Unix()
    pbReq := &pb.TransactionRequest{
        TransactionId: "",
        UserId:        req.UserID,
        Amount:        req.Amount,
        Timestamp:     now,
        MerchantId:    req.MerchantID,
        MerchantRisk:  req.MerchantRisk,
    }
    if req.DeviceID != nil { pbReq.DeviceId = *req.DeviceID }
    if req.IPAddress != nil { pbReq.IpAddress = *req.IPAddress }

    cctx, cancel := context.WithTimeout(ctx, 2*time.Second)
    defer cancel()
    resp, err := client.GetFraudScore(cctx, pbReq)
    if err != nil { return 0, 0, nil, err }

    return resp.GetFraudScore(), resp.GetConfidence(), resp.GetRiskFactors(), nil
}

func storeTransaction(txID string, t TransactionRequest, fraudScore float64, isFraud bool) error {
    _, err := pg.Exec(`INSERT INTO transactions (transaction_id, user_id, amount, timestamp, merchant_id, merchant_risk, fraud_score, is_fraud) VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`,
        txID, t.UserID, t.Amount, time.Now().UTC(), t.MerchantID, t.MerchantRisk, fraudScore, isFraud)
    return err
}

func sendToKafka(txID string, t TransactionRequest, fraudScore float64, isFraud bool) {
    if kafkaW == nil { return }
    payload := map[string]interface{}{
        "transaction_id": txID,
        "user_id": t.UserID,
        "amount": t.Amount,
        "fraud_score": fraudScore,
        "is_fraud": isFraud,
        "timestamp": time.Now().Unix(),
    }
    b, _ := json.Marshal(payload)
    _ = kafkaW.WriteMessages(ctx, kafka.Message{Value: b})
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(status)
    _ = json.NewEncoder(w).Encode(v)
}

func withCORS(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.Header().Set("Access-Control-Allow-Origin", "*")
        w.Header().Set("Access-Control-Allow-Methods", "GET,POST,OPTIONS")
        w.Header().Set("Access-Control-Allow-Headers", "Content-Type,Authorization")
        if r.Method == http.MethodOptions { w.WriteHeader(http.StatusNoContent); return }
        next.ServeHTTP(w, r)
    })
}

func main() {
    if err := initConnections(); err != nil {
        log.Fatalf("startup error: %v", err)
    }
    mux := http.NewServeMux()
    mux.HandleFunc("/", rootHandler)
    mux.HandleFunc("/health", healthHandler)
    mux.HandleFunc("/transactions/process", processTransactionHandler)
    mux.HandleFunc("/transactions/batch", batchProcessHandler)
    mux.HandleFunc("/transactions/", getTransactionHandler)
    mux.HandleFunc("/users/", func(w http.ResponseWriter, r *http.Request) {
        if strings.HasSuffix(r.URL.Path, "/risk-score") { userRiskHandler(w, r); return }
        http.NotFound(w, r)
    })
    mux.HandleFunc("/alerts", alertsHandler)

    addr := ":8000"
    log.Printf("Go Fraud API listening on %s", addr)
    srv := &http.Server{ Addr: addr, Handler: withCORS(mux), ReadTimeout: 15 * time.Second, WriteTimeout: 15 * time.Second }
    log.Fatal(srv.ListenAndServe())
}


