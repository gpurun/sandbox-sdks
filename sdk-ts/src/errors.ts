export class BoxApiError extends Error {
  readonly status: number;
  readonly code: string;

  constructor(message: string, status: number, code: string) {
    super(message);
    this.name = 'BoxApiError';
    this.status = status;
    this.code = code;
  }
}

export class BoxdError extends Error {
  readonly status: number;

  constructor(message: string, status: number) {
    super(message);
    this.name = 'BoxdError';
    this.status = status;
  }
}
