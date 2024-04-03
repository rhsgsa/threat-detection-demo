import React from 'react';
import Dashboard from './Dashboard2';
import { Routes, Route } from 'react-router-dom';

const Views = () => {
  return (
    <Routes>
      <Route index element={<Dashboard />} />
      <Route path="*" element={<div>404 Not Found</div>} />
    </Routes>
  );
};

export default Views;