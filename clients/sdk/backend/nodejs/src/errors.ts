/**
 * ApiError represents an HTTP error returned by the ProgressDB API.
 * It includes the HTTP `status` code and the parsed response `body`.
 */
export class ApiError extends Error {
  status: number;
  body: any;
  constructor(status: number, body: any) {
    super(`API error ${status}`);
    this.status = status;
    this.body = body;
    Object.setPrototypeOf(this, ApiError.prototype);
  }
}
