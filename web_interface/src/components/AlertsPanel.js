import React from 'react';

function AlertsPanel({ alerts }) {
  const getSeverityClass = (severity) => {
    switch (severity?.toLowerCase()) {
      case 'critical':
        return 'critical';
      case 'high':
        return 'high';
      case 'medium':
        return 'medium';
      case 'low':
        return 'low';
      default:
        return 'medium';
    }
  };

  const formatTimestamp = (timestamp) => {
    if (!timestamp) return 'Unknown';
    try {
      const date = new Date(timestamp);
      return date.toLocaleString();
    } catch {
      return 'Invalid date';
    }
  };

  if (!alerts || alerts.length === 0) {
    return (
      <div className="alerts-panel">
        <h2>Fraud Alerts</h2>
        <div className="message success">
          No active fraud alerts at the moment.
        </div>
      </div>
    );
  }

  return (
    <div className="alerts-panel">
      <h2>Fraud Alerts ({alerts.length})</h2>
      
      {alerts.map((alert, index) => (
        <div key={alert.id || index} className={`alert-item ${getSeverityClass(alert.severity)}`}>
          <div className="alert-header">
            <span className="alert-severity">{alert.severity || 'Unknown'}</span>
            <span className="alert-time">{formatTimestamp(alert.created_at)}</span>
          </div>
          
          <div className="alert-description">
            {alert.description || 'No description available'}
          </div>
          
          {alert.transaction_id && (
            <div style={{ marginTop: '0.5rem', fontSize: '0.9rem', color: '#666' }}>
              Transaction ID: {alert.transaction_id}
            </div>
          )}
          
          {alert.confidence_score && (
            <div style={{ marginTop: '0.5rem', fontSize: '0.9rem', color: '#666' }}>
              Confidence: {(alert.confidence_score * 100).toFixed(1)}%
            </div>
          )}
        </div>
      ))}
    </div>
  );
}

export default AlertsPanel; 