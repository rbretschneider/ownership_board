/**
 * Sends a secret (an access token) and clears it from the form before the
 * request settles, so it never lingers in component state, success or failure.
 * The secret is passed straight to `send`; nothing else keeps a copy.
 */
export async function submitSecret<T>(secret: string, clear: () => void, send: (secret: string) => Promise<T>): Promise<T> {
  const value = secret.trim();
  clear();
  return send(value);
}

/** Whether a URL would send a token across the network unencrypted. */
export function isPlainHttp(url: string): boolean {
  return url.trim().toLowerCase().startsWith("http://");
}
