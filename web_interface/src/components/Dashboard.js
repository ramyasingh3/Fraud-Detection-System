import React from 'react';

function Dashboard({ stats }) {
  const statCards = [
    {
      label: 'Total Transactions',
      value: stats.totalTransactions,
      color: '#667eea'
    },
    {
      label: 'Fraud Detected',
      value: stats.fraudDetected,
      color: '#f44336'
    },
    {
      label: 'Alerts Generated',
      value: stats.alertsGenerated,
      color: '#ff9800'
    },
    {
      label: 'Success Rate',
      value: stats.totalTransactions > 0 
        ? Math.round(((stats.totalTransactions - stats.fraudDetected) / stats.totalTransactions) * 100)
        : 0,
      suffix: '%',
      color: '#4caf50'
    }
  ];

  return (
    <div className="dashboard">
      {statCards.map((stat, index) => (
        <div key={index} className="stat-card">
          <div className="stat-number" style={{ color: stat.color }}>
            {stat.value}{stat.suffix || ''}
          </div>
          <div className="stat-label">{stat.label}</div>
        </div>
      ))}
      
      <div className="stat-card">
        <div className="stat-number">🚀</div>
        <div className="stat-label">Real-time Processing</div>
      </div>
      
      <div className="stat-card">
        <div className="stat-number">🔒</div>
        <div className="stat-label">Secure & Reliable</div>
      </div>
    </div>
  );
}

export default Dashboard; 