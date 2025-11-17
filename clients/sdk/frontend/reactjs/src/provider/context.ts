import React, { createContext } from 'react';
import type { ProgressClientContextValue } from '../types/provider';

export const ProgressClientContext = createContext<ProgressClientContextValue | null>(null);