export class AnakinScraperError extends Error {
  statusCode?: number;
  errorCode?: string;
  constructor(message: string, statusCode?: number, errorCode?: string) {
    super(message);
    this.name = 'AnakinScraperError';
    this.statusCode = statusCode;
    this.errorCode = errorCode;
  }
}
