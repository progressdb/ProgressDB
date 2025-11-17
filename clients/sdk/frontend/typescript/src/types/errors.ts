// Error response types for ProgressDB API

export type ApiErrorType = {
  error: string;
  message?: string;
  code?: string;
  details?: Record<string, any>;
};

export type ValidationErrorType = {
  field: string;
  message: string;
  value?: any;
};

export type ApiErrorResponseType = {
  error: ApiErrorType;
  validationErrors?: ValidationErrorType[];
};