// OAuth2 Token Validation Load Test
// Tests the performance of OAuth2 Bearer token validation
// SLA: p95 < 50ms, error rate < 1%

import http from 'k6/http';
import { check, sleep } from 'k6';
import { Rate, Trend } from 'k6/metrics';
import { SharedArray } from 'k6/data';
import { BASE_URL, API_VERSION, THRESHOLDS, SCENARIOS, OAUTH2_TOKEN_URL, OAUTH2_CLIENT_ID, OAUTH2_CLIENT_SECRET, OAUTH2_USERNAME, OAUTH2_PASSWORD } from '../config.js';
import { getOAuth2Token, createOAuth2Headers, validateOAuth2Auth } from '../helpers/auth.js';

// Custom metrics
const tokenValidationLatency = new Trend('token_validation_latency', true);
const tokenValidationErrors = new Rate('token_validation_errors');

// Shared OAuth2 token (obtained once and reused)
let accessToken;

// Test configuration
export const options = {
  scenarios: {
    oauth2_load: SCENARIOS.load,
  },
  thresholds: THRESHOLDS.oauth2,
};

export function setup() {
  console.log('Obtaining OAuth2 access token...');
  const token = getOAuth2Token(
    OAUTH2_TOKEN_URL,
    OAUTH2_CLIENT_ID,
    OAUTH2_CLIENT_SECRET,
    OAUTH2_USERNAME,
    OAUTH2_PASSWORD
  );

  if (!token) {
    throw new Error('Failed to obtain OAuth2 access token');
  }

  console.log('OAuth2 token obtained successfully');
  console.log(`Target: ${BASE_URL}${API_VERSION}`);
  return { token };
}

export default function (data) {
  // Use OAuth2 token from setup
  const headers = createOAuth2Headers(data.token);
  const params = {
    headers,
    tags: { scenario: 'oauth2' },
  };

  // Test authenticated API call with OAuth2 token
  const startTime = Date.now();
  const res = http.get(`${BASE_URL}${API_VERSION}/resourcePools`, params);
  const duration = Date.now() - startTime;

  // Record metrics
  tokenValidationLatency.add(duration);

  // Validate response
  const success = check(res, {
    'status is 200': (r) => r.status === 200,
    'response time < 50ms (p95 target)': (r) => r.timings.duration < 50,
    'OAuth2 token validated': (r) => validateOAuth2Auth(r),
    'response has data': (r) => r.body.length > 0,
  });

  tokenValidationErrors.add(!success);

  // Think time between requests
  sleep(1);
}

export function teardown(data) {
  console.log('OAuth2 authentication load test completed');
}
