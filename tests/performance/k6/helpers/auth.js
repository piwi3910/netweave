// Authentication helpers for k6 load tests
import http from 'k6/http';
import { check } from 'k6';

// Get OAuth2 access token
export function getOAuth2Token(tokenUrl, clientId, clientSecret, username, password) {
  const params = {
    headers: {
      'Content-Type': 'application/x-www-form-urlencoded',
    },
  };

  const payload = {
    grant_type: 'password',
    client_id: clientId,
    client_secret: clientSecret,
    username: username,
    password: password,
  };

  const body = Object.keys(payload)
    .map((key) => `${encodeURIComponent(key)}=${encodeURIComponent(payload[key])}`)
    .join('&');

  const res = http.post(tokenUrl, body, params);

  check(res, {
    'OAuth2 token obtained': (r) => r.status === 200,
    'OAuth2 response has token': (r) => JSON.parse(r.body).access_token !== undefined,
  });

  if (res.status !== 200) {
    console.error(`Failed to get OAuth2 token: ${res.status} ${res.body}`);
    return null;
  }

  const tokenData = JSON.parse(res.body);
  return tokenData.access_token;
}

// Create authorization header for OAuth2
export function createOAuth2Headers(token) {
  return {
    'Authorization': `Bearer ${token}`,
    'Content-Type': 'application/json',
  };
}

// Create parameters for mTLS requests
export function createMTLSParams(certPath, keyPath, caPath) {
  return {
    headers: {
      'Content-Type': 'application/json',
    },
    tls: {
      cert: certPath,
      key: keyPath,
      ca: caPath,
    },
  };
}

// Validate mTLS certificate authentication
export function validateMTLSAuth(res) {
  return check(res, {
    'mTLS auth successful': (r) => r.status !== 401 && r.status !== 403,
    'mTLS certificate accepted': (r) => !r.body.includes('certificate'),
  });
}

// Validate OAuth2 authentication
export function validateOAuth2Auth(res) {
  return check(res, {
    'OAuth2 auth successful': (r) => r.status !== 401 && r.status !== 403,
    'OAuth2 token accepted': (r) => !r.body.includes('Unauthorized') && !r.body.includes('token'),
  });
}

// Refresh OAuth2 token if expired
export function refreshTokenIfNeeded(token, tokenUrl, clientId, clientSecret) {
  // Parse JWT to check expiration
  try {
    const parts = token.split('.');
    if (parts.length !== 3) {
      return getOAuth2Token(tokenUrl, clientId, clientSecret);
    }

    const payload = JSON.parse(atob(parts[1]));
    const exp = payload.exp;
    const now = Date.now() / 1000;

    // Refresh if token expires in less than 5 minutes
    if (exp - now < 300) {
      return getOAuth2Token(tokenUrl, clientId, clientSecret);
    }

    return token;
  } catch (e) {
    console.error(`Failed to parse token: ${e}`);
    return getOAuth2Token(tokenUrl, clientId, clientSecret);
  }
}

// Helper to decode base64url (JWT payloads)
function atob(str) {
  return decodeURIComponent(
    Array.prototype.map
      .call(
        Buffer.from(str.replace(/-/g, '+').replace(/_/g, '/'), 'base64'),
        (c) => '%' + ('00' + c.charCodeAt(0).toString(16)).slice(-2)
      )
      .join('')
  );
}
