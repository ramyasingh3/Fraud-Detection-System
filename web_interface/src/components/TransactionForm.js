import React, { useState } from 'react';

function TransactionForm({ onTransactionProcessed }) {
  const [formData, setFormData] = useState({
    user_id: '',
    amount: '',
    merchant_id: '',
    merchant_risk: '',
    device_id: '',
    ip_address: ''
  });
  
  const [result, setResult] = useState(null);
  const [loading, setLoading] = useState(false);
  const [message, setMessage] = useState('');

  const userOptions = ['USER001', 'USER002', 'USER003'];
  const merchantOptions = ['M123', 'M456', 'M789', 'M987'];
  const deviceOptions = ['iphone-15', 'android-12', 'web-portal'];

  const handleInputChange = (e) => {
    const { name, value } = e.target;
    setFormData(prev => ({
      ...prev,
      [name]: value
    }));
  };

  const handleSubmit = async (e) => {
    e.preventDefault();
    setLoading(true);
    setMessage('');
    setResult(null);

    try {
      // Always go through the Nginx proxy to avoid CORS/mixed-origin issues
      const baseUrl = '/api';
      // Build payload with correct types and without empty optional fields
      const payload = {
        user_id: formData.user_id,
        amount: formData.amount !== '' ? parseFloat(formData.amount) : 0,
        merchant_id: formData.merchant_id,
        merchant_risk: formData.merchant_risk !== '' ? parseFloat(formData.merchant_risk) : 0,
      };
      if (formData.device_id) payload.device_id = formData.device_id;
      if (formData.ip_address) payload.ip_address = formData.ip_address;

      const response = await fetch(`${baseUrl}/transactions/process`, {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
        },
        body: JSON.stringify(payload),
      });

      if (!response.ok) {
        const errText = await response.text();
        throw new Error(errText || 'Failed to process transaction');
      }

      const data = await response.json();
      setResult(data);
      setMessage('Transaction processed successfully!');
      // omit graph/history
      
      // Reset form
      setFormData({
        user_id: '',
        amount: '',
        merchant_id: '',
        merchant_risk: '',
        device_id: '',
        ip_address: ''
      });
      
      // Notify parent component
      if (onTransactionProcessed) {
        onTransactionProcessed();
      }
      
    } catch (error) {
      setMessage('Error processing transaction: ' + error.message);
    } finally {
      setLoading(false);
    }
  };

  return (
    <div className="transaction-form">
      <h2 className="form-title">Process Transaction</h2>
      
      {message && (
        <div className={`message ${message.includes('Error') ? 'error' : 'success'}`}>
          {message}
        </div>
      )}
      
      {result && (() => {
        const scoreNum = Number(result.fraud_score);
        const confNum = Number(result.confidence);
        const scorePct = isNaN(scoreNum) ? null : Math.min(100, Math.max(0, scoreNum * 100));
        const confPct = isNaN(confNum) ? null : Math.min(100, Math.max(0, confNum * 100));
        const procMs = result.processing_time_ms ?? result.processing_time ?? null;
        return (
          <div className={`result ${result.is_fraud ? 'fraudulent' : 'legitimate'}`} style={{ marginTop: 16, marginBottom: 16 }}>
            <div style={{ fontWeight: 600, marginBottom: 8 }}>Result: {result.is_fraud ? 'Fraudulent' : 'Legitimate'}</div>
            <div style={{ marginBottom: 8 }}>
              Fraud Score: {scorePct !== null ? `${scorePct.toFixed(2)}%` : 'N/A'} (threshold 70%)
            </div>
            <div style={{ height: 10, background: '#eee', borderRadius: 6, overflow: 'hidden', marginBottom: 8 }}>
              <div style={{ width: `${scorePct !== null ? scorePct : 0}%`, height: '100%', background: result.is_fraud ? '#d32f2f' : '#2e7d32' }} />
            </div>
            <div style={{ marginBottom: 8 }}>Confidence: {confPct !== null ? `${confPct.toFixed(2)}%` : 'N/A'}</div>
            {result.risk_factors && result.risk_factors.length > 0 && (
              <div style={{ marginBottom: 8 }}>Risk Factors: {result.risk_factors.join(', ')}</div>
            )}
            <div>Processing Time: {procMs !== null ? `${procMs} ms` : 'N/A'}</div>
          </div>
        );
      })()}

      

      <form onSubmit={handleSubmit}>
        <div className="form-group">
          <label className="form-label">User ID *</label>
          <select
            name="user_id"
            className="form-input"
            value={formData.user_id}
            onChange={handleInputChange}
            required
          >
            <option value="" disabled>Select a user</option>
            {userOptions.map(u => (
              <option key={u} value={u}>{u}</option>
            ))}
          </select>
        </div>

        <div className="form-group">
          <label className="form-label">Amount ($) *</label>
          <input
            type="number"
            name="amount"
            className="form-input"
            value={formData.amount}
            onChange={handleInputChange}
            placeholder="Enter transaction amount"
            step="0.01"
            min="0"
            required
          />
        </div>

        <div className="form-group">
          <label className="form-label">Merchant ID *</label>
          <select
            name="merchant_id"
            className="form-input"
            value={formData.merchant_id}
            onChange={handleInputChange}
            required
          >
            <option value="" disabled>Select a merchant</option>
            {merchantOptions.map(m => (
              <option key={m} value={m}>{m}</option>
            ))}
          </select>
        </div>

        <div className="form-group">
          <label className="form-label">Merchant Risk (0-1) *</label>
          <input
            type="number"
            name="merchant_risk"
            className="form-input"
            value={formData.merchant_risk}
            onChange={handleInputChange}
            placeholder="Enter merchant risk score"
            step="0.01"
            min="0"
            max="1"
            required
          />
        </div>

        

        <div className="form-group">
          <label className="form-label">Device ID</label>
          <select
            name="device_id"
            className="form-input"
            value={formData.device_id}
            onChange={handleInputChange}
          >
            <option value="">Select a device (optional)</option>
            {deviceOptions.map(d => (
              <option key={d} value={d}>{d}</option>
            ))}
          </select>
        </div>

        <div className="form-group">
          <label className="form-label">IP Address</label>
          <input
            type="text"
            name="ip_address"
            className="form-input"
            value={formData.ip_address}
            onChange={handleInputChange}
            placeholder="Enter IP address (optional)"
          />
        </div>

        <button
          type="submit"
          className="submit-btn"
          disabled={loading}
        >
          {loading ? 'Processing...' : 'Process Transaction'}
        </button>
      </form>

      
    </div>
  );
}

export default TransactionForm; 