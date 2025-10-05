import React from 'react';

function Navigation({ activeTab, onTabChange }) {
  const tabs = [
    { id: 'dashboard', label: 'Dashboard' },
    { id: 'transactions', label: 'Process Transaction' },
    { id: 'alerts', label: 'Fraud Alerts' }
  ];

  return (
    <nav className="navigation">
      <ul className="nav-tabs">
        {tabs.map(tab => (
          <li
            key={tab.id}
            className={`nav-tab ${activeTab === tab.id ? 'active' : ''}`}
            onClick={() => onTabChange(tab.id)}
          >
            {tab.label}
          </li>
        ))}
      </ul>
    </nav>
  );
}

export default Navigation; 