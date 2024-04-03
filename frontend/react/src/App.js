import React from 'react';
import { BrowserRouter } from 'react-router-dom';
import Views from './Views';

function App() {
  return (
    <div className="App">
      <BrowserRouter>
        <Views />
      </BrowserRouter>
    </div>
  );
}

export default App;