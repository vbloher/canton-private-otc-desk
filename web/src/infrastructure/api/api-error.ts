import type { ApiFailure } from "@/application/errors";

/** A desk API error: the HTTP status, the body's `error` text and its optional `code`. */
export class ApiError extends Error implements ApiFailure {
  readonly status: number;
  readonly code?: string;

  constructor(status: number, message: string, code?: string, options?: { cause?: unknown }) {
    super(message, options);
    this.name = "ApiError";
    this.status = status;
    if (code !== undefined) this.code = code;
  }
}

const isErrorBody = (v: unknown): v is { error: string; code?: unknown } =>
  typeof v === "object" && v !== null && typeof (v as { error?: unknown }).error === "string";

function fallbackMessage(status: number): string {
  if (status === 401) return "Your session has ended — connect your wallet again.";
  if (status >= 502 && status <= 504) return `The desk is unavailable right now (${status}).`;
  return `Request failed (${status}).`;
}

/** The error a non-2xx answer stands for; the API's own wording when it sent one. */
export function apiErrorFrom(status: number, body: unknown): ApiError {
  if (isErrorBody(body) && body.error !== "") {
    return new ApiError(status, body.error, typeof body.code === "string" ? body.code : undefined);
  }
  return new ApiError(status, fallbackMessage(status));
}

/** fetch rejects with a TypeError when the request never got an answer (network down, refused, CORS). */
export function isNetworkFailure(e: unknown): e is TypeError {
  return e instanceof TypeError;
}

export function unreachable(cause: unknown): ApiError {
  return new ApiError(0, "Can't reach the desk — check your connection and that the desk is running.", undefined, {
    cause,
  });
}
