import React from 'react';
import './App.css';
import TransactionForm from './components/TransactionForm';

function App() {
  return (
    <div className="App" style={{ maxWidth: 720, margin: '0 auto', padding: 16 }}>
      <h1 style={{ marginBottom: 8, textAlign: 'center', color: '#2b2b2b' }}>Fraud Detection System</h1>
      <main className="main-content">
        <TransactionForm />
      </main>
    </div>
  );
}

export default App;