// k6 Performance Testing Configuration
// Shared configuration for all O2-IMS Gateway load tests

export const BASE_URL = __ENV.BASE_URL || 'https://localhost:8443';
export const API_VERSION = '/o2ims-infrastructureInventory/v1';

// SLA Thresholds (O-RAN Requirements)
export const THRESHOLDS = {
  // Authentication latency requirements
  auth: {
    'http_req_duration{scenario:auth}': [
      'p(95)<100',  // 95th percentile < 100ms
      'p(99)<500',  // 99th percentile < 500ms
    ],
    'http_req_failed{scenario:auth}': ['rate<0.01'], // < 1% error rate
  },

  // OAuth2 token validation requirements
  oauth2: {
    'http_req_duration{scenario:oauth2}': ['p(95)<50'], // 95th percentile < 50ms
    'http_req_failed{scenario:oauth2}': ['rate<0.01'],
  },

  // API CRUD operations requirements
  api: {
    'http_req_duration{scenario:api}': [
      'p(95)<100',  // 95th percentile < 100ms
      'p(99)<500',  // 99th percentile < 500ms
    ],
    'http_req_failed{scenario:api}': ['rate<0.01'],
  },

  // Subscription operations
  subscriptions: {
    'http_req_duration{scenario:subscriptions}': [
      'p(95)<100',
      'p(99)<500',
    ],
    'http_req_failed{scenario:subscriptions}': ['rate<0.01'],
  },
};

// Load Test Scenarios
export const SCENARIOS = {
  // Smoke test - validate basic functionality
  smoke: {
    executor: 'constant-vus',
    vus: 1,
    duration: '30s',
  },

  // Load test - sustained load
  load: {
    executor: 'constant-vus',
    vus: 50,
    duration: '5m',
  },

  // Stress test - gradual increase
  stress: {
    executor: 'ramping-vus',
    startVUs: 0,
    stages: [
      { duration: '2m', target: 100 },  // Ramp up to 100 users
      { duration: '5m', target: 100 },  // Stay at 100 users
      { duration: '2m', target: 200 },  // Ramp up to 200 users
      { duration: '5m', target: 200 },  // Stay at 200 users
      { duration: '2m', target: 300 },  // Ramp up to 300 users
      { duration: '5m', target: 300 },  // Stay at 300 users
      { duration: '2m', target: 0 },    // Ramp down
    ],
  },

  // Spike test - sudden load increase
  spike: {
    executor: 'ramping-vus',
    startVUs: 0,
    stages: [
      { duration: '10s', target: 100 },   // Baseline
      { duration: '10s', target: 1000 },  // Spike!
      { duration: '1m', target: 1000 },   // Hold spike
      { duration: '10s', target: 100 },   // Back to baseline
      { duration: '1m', target: 100 },    // Recovery
    ],
  },

  // Soak test - extended duration
  soak: {
    executor: 'constant-vus',
    vus: 50,
    duration: '1h',
  },
};

// Certificate paths for mTLS authentication
export const MTLS_CERT = __ENV.MTLS_CERT || './certs/client.crt';
export const MTLS_KEY = __ENV.MTLS_KEY || './certs/client.key';
export const CA_CERT = __ENV.CA_CERT || './certs/ca.crt';

// OAuth2 configuration
export const OAUTH2_TOKEN_URL = __ENV.OAUTH2_TOKEN_URL || 'https://keycloak:8080/realms/o2ims/protocol/openid-connect/token';
export const OAUTH2_CLIENT_ID = __ENV.OAUTH2_CLIENT_ID || 'netweave-client';
export const OAUTH2_CLIENT_SECRET = __ENV.OAUTH2_CLIENT_SECRET || '';
export const OAUTH2_USERNAME = __ENV.OAUTH2_USERNAME || 'testuser';
export const OAUTH2_PASSWORD = __ENV.OAUTH2_PASSWORD || 'testpass';

// Rate limiting configuration
export const RATE_LIMIT_RPS = parseInt(__ENV.RATE_LIMIT_RPS || '1000', 10);

// Test data configuration
export const TEST_TENANT_ID = __ENV.TEST_TENANT_ID || 'tenant-001';
export const TEST_POOL_ID = __ENV.TEST_POOL_ID || 'pool-001';

// Options for test output
export const OUTPUT_OPTIONS = {
  discardResponseBodies: false,
};
