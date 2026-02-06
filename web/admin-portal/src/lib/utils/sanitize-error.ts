const SENSITIVE_PATTERNS = [
  /at\s+\S+\s+\(\S+:\d+:\d+\)/g, // Stack traces
  /\/[\w/.-]+\.(ts|js|go|py):\d+/g, // File paths with line numbers
  /(?:password|secret|token|key|auth)\s*[:=]\s*\S+/gi, // Credentials
  /\b(?:\d{1,3}\.){3}\d{1,3}(?::\d+)?\b/g, // IP addresses
  /(?:redis|postgres|mysql|mongo):\/\/\S+/gi, // Connection strings
];

export function sanitizeError(message: string): string {
  if (!message || typeof message !== "string") {
    return "An unexpected error occurred";
  }

  let sanitized = message;

  for (const pattern of SENSITIVE_PATTERNS) {
    sanitized = sanitized.replace(pattern, "[redacted]");
  }

  // Truncate overly long messages
  if (sanitized.length > 200) {
    sanitized = sanitized.slice(0, 200) + "...";
  }

  return sanitized;
}

export function getDisplayError(err: unknown): string {
  if (err instanceof Error) {
    return sanitizeError(err.message);
  }
  if (
    typeof err === "object" &&
    err !== null &&
    "message" in err &&
    typeof (err as Record<string, unknown>).message === "string"
  ) {
    return sanitizeError((err as Record<string, unknown>).message as string);
  }
  return "An unexpected error occurred";
}
