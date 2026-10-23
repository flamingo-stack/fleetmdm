/**
 * This contains a collection of utility functions for working with
 * users auth token.
 */
import Cookie from "js-cookie";

const DEFAULT_EXPIRATION_DAYS = 5;

// >>> OPENFRAME(auth-token-http-fallback): allow non-secure cookie fallback for non-TLS deployments — openframe/docs/auth.md
// The `__Host-` cookie name prefix and the `Secure` attribute both require the
// cookie to be set from a secure (HTTPS) context. When Fleet is served over
// plain HTTP (e.g. a Docker deployment without TLS), the browser silently
// refuses to store such a cookie, leaving the user unable to authenticate
// because the token is never persisted and therefore never attached to
// subsequent requests. Detect the context and fall back to a regular,
// non-secure cookie when not served over HTTPS.
const isSecure = (): boolean => window.location.protocol === "https:";

// `__Host-` prefixed names are only valid on secure cookies, so the cookie name
// must match the context it was stored in for get/remove to find it.
const getTokenName = (): string => (isSecure() ? "__Host-token" : "token");
// <<< OPENFRAME(auth-token-http-fallback)

const save = (token: string, expiresAt?: Date): void => {
  // >>> OPENFRAME(auth-token-http-fallback): allow non-secure cookie fallback for non-TLS deployments — openframe/docs/auth.md
  Cookie.set(getTokenName(), token, {
    secure: isSecure(),
    sameSite: "lax",
    expires: expiresAt ?? DEFAULT_EXPIRATION_DAYS,
  });
  // <<< OPENFRAME(auth-token-http-fallback)
};

const get = (): string | null => {
  // >>> OPENFRAME(auth-token-http-fallback): allow non-secure cookie fallback for non-TLS deployments — openframe/docs/auth.md
  return Cookie.get(getTokenName()) || null;
  // <<< OPENFRAME(auth-token-http-fallback)
};

const remove = (): void => {
  // NOTE: the secure and sameSite from the cookie must be provided
  // to correctly remove. That is why we include the options here as well.
  // >>> OPENFRAME(auth-token-http-fallback): allow non-secure cookie fallback for non-TLS deployments — openframe/docs/auth.md
  Cookie.remove(getTokenName(), {
    secure: isSecure(),
    sameSite: "lax",
  });
  // <<< OPENFRAME(auth-token-http-fallback)
};

export default {
  save,
  get,
  remove,
};
