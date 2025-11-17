// Error response types for ProgressDB API

export type ApiError = {
  error: string;
  message?: string;
  code?: string;
  details?: Record<string, any>;
};

export type ValidationError = {
  field: string;
  message: string;
  value?: any;
};

export type ApiErrorResponse = {
  error: ApiError;
  validationErrors?: ValidationError[];
};