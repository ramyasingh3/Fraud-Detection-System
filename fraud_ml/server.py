import grpc
import asyncio
import pickle
import numpy as np
import pandas as pd
from sklearn.ensemble import RandomForestClassifier
from sklearn.preprocessing import StandardScaler
import psycopg2
import redis
import json
import os
from concurrent import futures
import logging

# Import generated gRPC code (you'll need to generate this)
# from fraud_detection_pb2 import *
# from fraud_detection_pb2_grpc import *

# Configure logging
logging.basicConfig(level=logging.INFO)
logger = logging.getLogger(__name__)

# Configuration
POSTGRES_HOST = os.getenv('POSTGRES_HOST', 'localhost')
POSTGRES_DB = os.getenv('POSTGRES_DB', 'fraud_detection')
POSTGRES_USER = os.getenv('POSTGRES_USER', 'fraud_user')
POSTGRES_PASSWORD = os.getenv('POSTGRES_PASSWORD', 'fraud_password')
REDIS_HOST = os.getenv('REDIS_HOST', 'localhost')
REDIS_PORT = int(os.getenv('REDIS_PORT', 6379))

class FraudDetectionMLServicer:
    def __init__(self):
        self.model = None
        self.scaler = None
        self.features = None
        self.model_version = "1.0.0"
        self.load_model()
        
        # Initialize connections
        self.redis_client = redis.Redis(host=REDIS_HOST, port=REDIS_PORT, decode_responses=True)
        self.postgres_conn = psycopg2.connect(
            host=POSTGRES_HOST,
            database=POSTGRES_DB,
            user=POSTGRES_USER,
            password=POSTGRES_PASSWORD
        )
    
    def load_model(self):
        """Load the trained fraud detection model"""
        try:
            with open('model.pkl', 'rb') as f:
                self.model = pickle.load(f)
            with open('scaler.pkl', 'rb') as f:
                self.scaler = pickle.load(f)
            self.features = ['amount', 'merchant_risk', 'user_risk_score', 'amount_to_history_ratio']
            logger.info(f"Model loaded successfully. Version: {self.model_version}")
        except FileNotFoundError:
            logger.warning("Model files not found. Training new model...")
            self.train_new_model()
    
    def train_new_model(self):
        """Train a new fraud detection model"""
        # Generate sample data
        np.random.seed(42)
        n_samples = 10000
        
        data = {
            'amount': np.random.uniform(1, 10000, n_samples),
            'merchant_risk': np.random.uniform(0, 1, n_samples),
            'user_risk_score': np.random.uniform(0, 1, n_samples),
            'amount_to_history_ratio': np.random.uniform(0.1, 10, n_samples),
            'is_fraud': np.random.choice([0, 1], n_samples, p=[0.8, 0.2])  # 20% fraud
        }
        
        df = pd.DataFrame(data)
        
        # Feature engineering
        X = df[self.features]
        y = df['is_fraud']
        
        # Scale features
        self.scaler = StandardScaler()
        X_scaled = self.scaler.fit_transform(X)
        
        # Train model
        self.model = RandomForestClassifier(n_estimators=100, random_state=42)
        self.model.fit(X_scaled, y)
        
        # Save model
        with open('model.pkl', 'wb') as f:
            pickle.dump(self.model, f)
        with open('scaler.pkl', 'wb') as f:
            pickle.dump(self.scaler, f)
        
        logger.info("New model trained and saved successfully")
    
    async def ProcessTransaction(self, request, context):
        """Process a transaction and return fraud detection result"""
        try:
            start_time = asyncio.get_event_loop().time()
            
            # Extract features from request
            features = np.array([[
                request.amount,
                request.merchant_risk,
                self.get_user_risk_score(request.user_id),
                self.get_amount_to_history_ratio(request.user_id, request.amount)
            ]])
            
            # Scale features
            features_scaled = self.scaler.transform(features)
            
            # Make prediction
            prediction = self.model.predict(features_scaled)[0]
            fraud_score = self.model.predict_proba(features_scaled)[0][1]
            
            # Determine risk factors
            risk_factors = self.identify_risk_factors(features[0], fraud_score)
            
            # Calculate confidence
            confidence = self.calculate_confidence(features_scaled[0])
            
            # Store transaction in database
            self.store_transaction(request, fraud_score, prediction)
            
            # Cache result
            self.cache_result(request.transaction_id, fraud_score, prediction)
            
            processing_time = int((asyncio.get_event_loop().time() - start_time) * 1000)
            
            # Return response (placeholder - would use actual gRPC response)
            return {
                'transaction_id': request.transaction_id,
                'is_fraud': bool(prediction),
                'fraud_score': float(fraud_score),
                'confidence': float(confidence),
                'risk_factors': risk_factors,
                'model_version': self.model_version,
                'processing_time_ms': processing_time
            }
            
        except Exception as e:
            logger.error(f"Error processing transaction: {e}")
            context.set_code(grpc.StatusCode.INTERNAL)
            context.set_details(f"Internal error: {str(e)}")
            return None
    
    async def GetFraudScore(self, request, context):
        """Get fraud score for a transaction"""
        try:
            # Extract features
            features = np.array([[
                request.amount,
                request.merchant_risk,
                self.get_user_risk_score(request.user_id),
                self.get_amount_to_history_ratio(request.user_id, request.amount)
            ]])
            
            # Scale and predict
            features_scaled = self.scaler.transform(features)
            fraud_score = self.model.predict_proba(features_scaled)[0][1]
            
            # Identify risk factors
            risk_factors = self.identify_risk_factors(features[0], fraud_score)
            
            # Calculate confidence
            confidence = self.calculate_confidence(features_scaled[0])
            
            # Return response (placeholder)
            return {
                'fraud_score': float(fraud_score),
                'confidence': float(confidence),
                'risk_factors': risk_factors
            }
            
        except Exception as e:
            logger.error(f"Error getting fraud score: {e}")
            context.set_code(grpc.StatusCode.INTERNAL)
            context.set_details(f"Internal error: {str(e)}")
            return None
    
    async def StreamFraudAlerts(self, request, context):
        """Stream real-time fraud alerts"""
        try:
            # Get alerts from database
            alerts = self.get_fraud_alerts(request.user_id, request.since_timestamp, request.max_alerts)
            
            for alert in alerts:
                # Yield each alert (placeholder)
                yield {
                    'alert_id': alert['id'],
                    'transaction_id': alert['transaction_id'],
                    'alert_type': alert['alert_type'],
                    'severity': alert['severity'],
                    'description': alert['description'],
                    'confidence_score': float(alert['confidence_score']),
                    'timestamp': alert['created_at'].timestamp()
                }
                
                await asyncio.sleep(0.1)  # Small delay between alerts
                
        except Exception as e:
            logger.error(f"Error streaming fraud alerts: {e}")
            context.set_code(grpc.StatusCode.INTERNAL)
            context.set_details(f"Internal error: {str(e)}")
    
    async def BatchProcessTransactions(self, request, context):
        """Process multiple transactions in batch"""
        try:
            start_time = asyncio.get_event_loop().time()
            responses = []
            
            for transaction in request.transactions:
                # Process each transaction
                features = np.array([[
                    transaction.amount,
                    transaction.merchant_risk,
                    self.get_user_risk_score(transaction.user_id),
                    self.get_amount_to_history_ratio(transaction.user_id, transaction.amount)
                ]])
                
                features_scaled = self.scaler.transform(features)
                prediction = self.model.predict(features_scaled)[0]
                fraud_score = self.model.predict_proba(features_scaled)[0][1]
                
                response = {
                    'transaction_id': transaction.transaction_id,
                    'is_fraud': bool(prediction),
                    'fraud_score': float(fraud_score),
                    'confidence': 0.8,  # Placeholder
                    'risk_factors': ["batch_processing"],
                    'model_version': self.model_version,
                    'processing_time_ms': 0
                }
                responses.append(response)
            
            total_time = int((asyncio.get_event_loop().time() - start_time) * 1000)
            
            return {
                'responses': responses,
                'total_processing_time_ms': total_time
            }
            
        except Exception as e:
            logger.error(f"Error in batch processing: {e}")
            context.set_code(grpc.StatusCode.INTERNAL)
            context.set_details(f"Internal error: {str(e)}")
            return None
    
    # Helper methods
    def get_user_risk_score(self, user_id: str) -> float:
        """Get user risk score from database"""
        try:
            with self.postgres_conn.cursor() as cur:
                cur.execute("SELECT risk_score FROM users WHERE user_id = %s", (user_id,))
                result = cur.fetchone()
                return result[0] if result else 0.5
        except Exception as e:
            logger.error(f"Error getting user risk score: {e}")
            return 0.5
    
    def get_amount_to_history_ratio(self, user_id: str, amount: float) -> float:
        """Calculate amount to history ratio"""
        try:
            with self.postgres_conn.cursor() as cur:
                cur.execute(
                    "SELECT AVG(amount) FROM transactions WHERE user_id = %s",
                    (user_id,)
                )
                result = cur.fetchone()
                avg_amount = result[0] if result and result[0] else 100.0
                return amount / avg_amount if avg_amount > 0 else 1.0
        except Exception as e:
            logger.error(f"Error calculating amount ratio: {e}")
            return 1.0
    
    def identify_risk_factors(self, features: np.ndarray, fraud_score: float) -> list:
        """Identify risk factors based on features and score"""
        risk_factors = []
        
        if features[0] > 5000:  # High amount
            risk_factors.append("high_amount")
        if features[1] > 0.8:  # High merchant risk
            risk_factors.append("high_merchant_risk")
        if features[2] > 0.7:  # High user risk
            risk_factors.append("high_user_risk")
        if features[3] > 5:  # High amount to history ratio
            risk_factors.append("unusual_amount_pattern")
        
        return risk_factors
    
    def calculate_confidence(self, features_scaled: np.ndarray) -> float:
        """Calculate confidence score based on feature values"""
        # Simple confidence calculation based on feature distances from mean
        confidence = 0.8  # Base confidence
        feature_std = np.std(features_scaled)
        confidence = max(0.5, confidence - feature_std * 0.1)
        return min(1.0, confidence)
    
    def store_transaction(self, request, fraud_score: float, is_fraud: bool):
        """Store transaction in database"""
        try:
            with self.postgres_conn.cursor() as cur:
                cur.execute("""
                    INSERT INTO transactions (transaction_id, user_id, amount, timestamp, 
                                           merchant_id, merchant_risk, fraud_score, is_fraud)
                    VALUES (%s, %s, %s, %s, %s, %s, %s, %s)
                """, (
                    request.transaction_id, request.user_id, request.amount, 
                    request.timestamp, request.merchant_id, request.merchant_risk,
                    fraud_score, is_fraud
                ))
                self.postgres_conn.commit()
        except Exception as e:
            logger.error(f"Error storing transaction: {e}")
    
    def cache_result(self, transaction_id: str, fraud_score: float, is_fraud: bool):
        """Cache fraud detection result"""
        try:
            cache_key = f"fraud_result:{transaction_id}"
            result = {
                'fraud_score': fraud_score,
                'is_fraud': is_fraud,
                'timestamp': int(asyncio.get_event_loop().time())
            }
            self.redis_client.setex(cache_key, 3600, json.dumps(result))  # Cache for 1 hour
        except Exception as e:
            logger.error(f"Error caching result: {e}")
    
    def get_fraud_alerts(self, user_id: str, since_timestamp: int, max_alerts: int):
        """Get fraud alerts from database"""
        try:
            with self.postgres_conn.cursor(cursor_factory=psycopg2.extras.RealDictCursor) as cur:
                cur.execute("""
                    SELECT * FROM fraud_alerts 
                    WHERE user_id = %s AND created_at > %s 
                    ORDER BY created_at DESC LIMIT %s
                """, (user_id, since_timestamp, max_alerts))
                return cur.fetchall()
        except Exception as e:
            logger.error(f"Error getting fraud alerts: {e}")
            return []

async def serve():
    """Start the gRPC server"""
    server = grpc.aio.server(futures.ThreadPoolExecutor(max_workers=10))
    
    # Add servicer (placeholder - would use actual gRPC service)
    # fraud_detection_pb2_grpc.add_FraudDetectionServiceServicer_to_server(
    #     FraudDetectionMLServicer(), server
    # )
    
    listen_addr = '[::]:50051'
    server.add_insecure_port(listen_addr)
    
    logger.info(f"Starting gRPC server on {listen_addr}")
    await server.start()
    await server.wait_for_termination()

if __name__ == '__main__':
    asyncio.run(serve()) 