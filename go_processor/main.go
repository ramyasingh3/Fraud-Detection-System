package main

import (
    "context"
    "database/sql"
    "encoding/json"
    "fmt"
    "log"
    "os"
    "strings"
    "time"

    _ "github.com/lib/pq"
    "github.com/segmentio/kafka-go"
    "github.com/go-redis/redis/v8"
)

type TransactionMessage struct {
    TransactionID string  `json:"transaction_id"`
    UserID        string  `json:"user_id"`
    Amount        float64 `json:"amount"`
    FraudScore    float64 `json:"fraud_score"`
    IsFraud       bool    `json:"is_fraud"`
    Timestamp     int64   `json:"timestamp"`
    DeviceID      *string `json:"device_id,omitempty"`
    IPAddress     *string `json:"ip_address,omitempty"`
}

var (
    ctx = context.Background()
    pg  *sql.DB
    rdb *redis.Client
)

func getenv(key, def string) string {
    if v := os.Getenv(key); v != "" { return v }
    return def
}

func initConnections() error {
    // Postgres
    pgHost := getenv("POSTGRES_HOST", "localhost")
    pgDB := getenv("POSTGRES_DB", "fraud_detection")
    pgUser := getenv("POSTGRES_USER", "fraud_user")
    pgPass := getenv("POSTGRES_PASSWORD", "fraud_password")
    dsn := "host=" + pgHost + " dbname=" + pgDB + " user=" + pgUser + " password=" + pgPass + " sslmode=disable"
    var err error
    pg, err = sql.Open("postgres", dsn)
    if err != nil { return err }
    if err = pg.Ping(); err != nil { return err }

    // Redis
    redisHost := getenv("REDIS_HOST", "localhost")
    redisPort := getenv("REDIS_PORT", "6379")
    rdb = redis.NewClient(&redis.Options{ Addr: redisHost+":"+redisPort })
    if err := rdb.Ping(ctx).Err(); err != nil { return err }
    return nil
}

func main() {
    if err := initConnections(); err != nil {
        log.Fatalf("startup error: %v", err)
    }

    brokers := strings.Split(getenv("KAFKA_BOOTSTRAP_SERVERS", "localhost:9092"), ",")
    reader := kafka.NewReader(kafka.ReaderConfig{
        Brokers:  brokers,
        GroupID:  "fraud-processor-group-go",
        Topic:    "fraud-transactions",
        MinBytes: 1,
        MaxBytes: 10e6,
    })
    defer reader.Close()

    alertWriter := &kafka.Writer{ Addr: kafka.TCP(brokers...), Topic: "fraud-alerts", Balancer: &kafka.LeastBytes{} }
    defer alertWriter.Close()

    log.Println("Go Transaction Processor started")
    for {
        m, err := reader.ReadMessage(ctx)
        if err != nil { log.Printf("read error: %v", err); time.Sleep(time.Second); continue }
        var tx TransactionMessage
        if err := json.Unmarshal(m.Value, &tx); err != nil { log.Printf("decode error: %v", err); continue }
        process(tx, alertWriter)
    }
}

func process(tx TransactionMessage, alertWriter *kafka.Writer) {
    // Update user risk score
    updateUserRiskScore(tx)
    // Store metadata
    storeMetadata(tx)
    // Update feature store
    updateFeatureStore(tx)
    // Cache recent transaction
    cacheRecent(tx)
    // Generate alert if needed
    if tx.IsFraud { generateAlert(tx, alertWriter) }
}

func updateUserRiskScore(tx TransactionMessage) {
    var current float64 = 0.5
    _ = pg.QueryRow(`SELECT risk_score FROM users WHERE user_id = $1`, tx.UserID).Scan(&current)
    adjustment := 0.0
    if tx.IsFraud { adjustment += 0.1 }
    if tx.FraudScore > 0.8 { adjustment += 0.05 }
    if tx.Amount > 5000 { adjustment += 0.03 }
    if !tx.IsFraud && tx.FraudScore < 0.3 { adjustment -= 0.02 }
    newRisk := current + adjustment
    if newRisk < 0 { newRisk = 0 }
    if newRisk > 1 { newRisk = 1 }
    _, _ = pg.Exec(`UPDATE users SET risk_score = $1, updated_at = CURRENT_TIMESTAMP WHERE user_id = $2`, newRisk, tx.UserID)
    _ = rdb.Set(ctx, "user_risk:"+tx.UserID, newRisk, time.Hour).Err()
}

func storeMetadata(tx TransactionMessage) {
    _, _ = pg.Exec(`UPDATE transactions SET device_id = $1, ip_address = $2 WHERE transaction_id = $3`, tx.DeviceID, tx.IPAddress, tx.TransactionID)
}

func updateFeatureStore(tx TransactionMessage) {
    now := time.Now()
    _, _ = pg.Exec(`INSERT INTO feature_store (user_id, feature_name, feature_value, feature_timestamp) VALUES ($1,$2,$3,$4)`, tx.UserID, "transaction_amount", tx.Amount, now)
    _, _ = pg.Exec(`INSERT INTO feature_store (user_id, feature_name, feature_value, feature_timestamp) VALUES ($1,$2,$3,$4)`, tx.UserID, "fraud_score", tx.FraudScore, now)
}

func cacheRecent(tx TransactionMessage) {
    key := "recent_transaction:" + tx.TransactionID
    b, _ := json.Marshal(tx)
    _ = rdb.Set(ctx, key, string(b), 30*time.Minute).Err()
    listKey := "user_recent_transactions:" + tx.UserID
    // Prepend tx id, trim to last 10
    _ = rdb.LPush(ctx, listKey, tx.TransactionID).Err()
    _ = rdb.LTrim(ctx, listKey, 0, 9).Err()
    _ = rdb.Expire(ctx, listKey, time.Hour).Err()
}

func generateAlert(tx TransactionMessage, alertWriter *kafka.Writer) {
    severity := "MEDIUM"
    if tx.FraudScore > 0.9 { severity = "CRITICAL" } else if tx.FraudScore > 0.8 { severity = "HIGH" }
    alertID := "ALERT_" + strconvFormat(time.Now().Unix()) + "_" + shortID(tx.TransactionID)
    description := "Fraud detected for transaction " + tx.TransactionID
    _, _ = pg.Exec(`INSERT INTO fraud_alerts (alert_id, transaction_id, alert_type, severity, description, confidence_score, status) VALUES ($1,$2,$3,$4,$5,$6,$7)`,
        alertID, tx.TransactionID, "FRAUD_DETECTED", severity, description, tx.FraudScore, "OPEN")
    payload := map[string]interface{}{
        "alert_id": alertID,
        "transaction_id": tx.TransactionID,
        "user_id": tx.UserID,
        "alert_type": "FRAUD_DETECTED",
        "severity": severity,
        "description": description,
        "fraud_score": tx.FraudScore,
        "timestamp": time.Now().Unix(),
    }
    b, _ := json.Marshal(payload)
    _ = alertWriter.WriteMessages(ctx, kafka.Message{Value: b})
}

func shortID(id string) string {
    if len(id) <= 8 { return id }
    return id[:8]
}

func strconvFormat(v int64) string { return fmt.Sprintf("%d", v) }


