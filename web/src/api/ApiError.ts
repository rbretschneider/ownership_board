/** An error returned by the API, carrying the envelope's code and message. */
export class ApiError extends Error {
  /** Machine-readable error code, e.g. `not_found`. */
  public readonly code: string;
  /** HTTP status of the failed response (0 for network failures). */
  public readonly status: number;

  /**
   * @param status HTTP status code (0 when the request never completed).
   * @param code Error code from the envelope.
   * @param message Human-readable message from the envelope.
   * @param cause The underlying error, when there is one.
   */
  constructor(status: number, code: string, message: string, cause?: unknown) {
    super(message, cause === undefined ? undefined : { cause });
    this.name = "ApiError";
    this.status = status;
    this.code = code;
  }
}
