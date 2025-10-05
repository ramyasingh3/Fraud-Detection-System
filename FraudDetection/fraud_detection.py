# fraud_detection.py
import pandas as pd
import numpy as np
from sklearn.model_selection import train_test_split
from sklearn.ensemble import RandomForestClassifier
from sklearn.preprocessing import StandardScaler
from sklearn.metrics import classification_report
from flask import Flask, render_template, request, jsonify
import pickle

# --- Simulated Data Generation (since we don’t have real transaction data) ---
def generate_sample_data(n_samples=1000):
    np.random.seed(42)
    data = {
        'amount': np.random.uniform(1, 1000, n_samples),
        'time': np.random.uniform(0, 24, n_samples),
        'merchant_risk': np.random.uniform(0, 1, n_samples),
        'user_history': np.random.randint(1, 100, n_samples),
        'is_fraud': np.random.choice([0, 1], n_samples, p=[0.5, 0.5])  # 50% fraud
    }
    return pd.DataFrame(data)

# --- Data Preprocessing and Model Training ---
def preprocess_and_train():
    # Generate or load data
    df = generate_sample_data()
    
    # Feature engineering
    df['amount_to_history_ratio'] = df['amount'] / df['user_history']
    features = ['amount', 'time', 'merchant_risk', 'user_history', 'amount_to_history_ratio']
    X = df[features]
    y = df['is_fraud']
    
    # Scale features
    scaler = StandardScaler()
    X_scaled = scaler.fit_transform(X)
    
    # Split data
    X_train, X_test, y_train, y_test = train_test_split(X_scaled, y, test_size=0.2, random_state=42)
    
    # Train model
    model = RandomForestClassifier(n_estimators=100, random_state=42)
    model.fit(X_train, y_train)
    
    # Evaluate
    y_pred = model.predict(X_test)
    print("Model Performance:\n", classification_report(y_test, y_pred))
    
    # Save model and scaler
    with open('model.pkl', 'wb') as f:
        pickle.dump(model, f)
    with open('scaler.pkl', 'wb') as f:
        pickle.dump(scaler, f)
    
    return model, scaler, features

# --- Flask App ---
app = Flask(__name__)

# Load model and scaler
try:
    with open('model.pkl', 'rb') as f:
        model = pickle.load(f)
    with open('scaler.pkl', 'rb') as f:
        scaler = pickle.load(f)
except:
    model, scaler, features = preprocess_and_train()

# Define features used in the model
features = ['amount', 'time', 'merchant_risk', 'user_history', 'amount_to_history_ratio']

@app.route('/')
def home():
    return render_template('index.html')

@app.route('/predict', methods=['POST'])
@app.route('/predict', methods=['POST'])
@app.route('/predict', methods=['POST'])
def predict():
    data = request.form.to_dict()
    amount = float(data['amount'])
    time = float(data['time'])
    merchant_risk = float(data['merchant_risk'])
    user_history = float(data['user_history'])
    
    amount_to_history_ratio = amount / user_history
    
    input_data = np.array([[amount, time, merchant_risk, user_history, amount_to_history_ratio]])
    input_scaled = scaler.transform(input_data)
    
    prediction = model.predict(input_scaled)[0]
    probability = model.predict_proba(input_scaled)[0][1]
    
    # Additional rule: High amount + low history = suspicious
    if amount > 800 and user_history < 5:
        result = 'Fraudulent'
        probability = max(probability, 0.9)  # Boost probability
    else:
        result = 'Fraudulent' if prediction == 1 else 'Legitimate'
    
    return jsonify({'result': result, 'probability': round(probability * 100, 2)})

# --- Main Execution ---
if __name__ == '__main__':
    # Train model if not already trained
    try:
        with open('model.pkl', 'rb') as f:
            pass
    except:
        preprocess_and_train()
    
    # Run Flask app
    app.run(debug=True, port=5001)