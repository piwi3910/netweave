// API CRUD Operations Load Test
// Tests full lifecycle operations under load: Create, Read, Update, Delete
// Validates database performance, transaction handling, and consistency
// SLA: p95 < 100ms, p99 < 500ms, error rate < 1%

import http from 'k6/http';
import { check, sleep } from 'k6';
import { Rate, Trend, Counter } from 'k6/metrics';
import { BASE_URL, API_VERSION, THRESHOLDS, SCENARIOS, TEST_TENANT_ID, OAUTH2_TOKEN_URL, OAUTH2_CLIENT_ID, OAUTH2_CLIENT_SECRET, OAUTH2_USERNAME, OAUTH2_PASSWORD } from '../config.js';
import { getOAuth2Token, createOAuth2Headers } from '../helpers/auth.js';

// Custom metrics
const createLatency = new Trend('create_latency', true);
const readLatency = new Trend('read_latency', true);
const updateLatency = new Trend('update_latency', true);
const deleteLatency = new Trend('delete_latency', true);
const crudErrors = new Rate('crud_errors');
const operationCounter = new Counter('operations_by_type');

// Test configuration
export const options = {
  scenarios: {
    crud_load: SCENARIOS.load,
  },
  thresholds: {
    ...THRESHOLDS.api,
    'create_latency': ['p(95)<100', 'p(99)<500'],
    'read_latency': ['p(95)<100', 'p(99)<500'],
    'update_latency': ['p(95)<100', 'p(99)<500'],
    'delete_latency': ['p(95)<100', 'p(99)<500'],
    'crud_errors': ['rate<0.01'],
  },
};

export function setup() {
  console.log('Setting up CRUD operations test...');
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

  console.log('CRUD test ready');
  console.log(`Target: ${BASE_URL}${API_VERSION}`);
  return { token };
}

export default function (data) {
  const headers = createOAuth2Headers(data.token);
  const params = {
    headers: {
      ...headers,
      'Content-Type': 'application/json',
    },
    tags: { scenario: 'api' },
  };

  // Generate unique ID for this VU and iteration
  const resourceId = `crud-test-${__VU}-${__ITER}`;

  // CREATE: Create a new subscription
  const createPayload = {
    callback: `https://smo.example.com/notify/${resourceId}`,
    consumerSubscriptionId: resourceId,
    filter: '(eq,resourceTypeId,node)',
  };

  let startTime = Date.now();
  let createRes = http.post(
    `${BASE_URL}${API_VERSION}/subscriptions`,
    JSON.stringify(createPayload),
    params
  );
  createLatency.add(Date.now() - startTime);
  operationCounter.add(1, { operation: 'create' });

  const createSuccess = check(createRes, {
    'CREATE: status is 201': (r) => r.status === 201,
    'CREATE: response has ID': (r) => {
      try {
        const body = JSON.parse(r.body);
        return body.subscriptionId !== undefined;
      } catch {
        return false;
      }
    },
  });

  if (!createSuccess) {
    crudErrors.add(1);
    return; // Skip rest if create failed
  }

  const subscription = JSON.parse(createRes.body);
  const subscriptionId = subscription.subscriptionId;

  sleep(0.1);

  // READ: Get the created subscription
  startTime = Date.now();
  const readRes = http.get(
    `${BASE_URL}${API_VERSION}/subscriptions/${subscriptionId}`,
    params
  );
  readLatency.add(Date.now() - startTime);
  operationCounter.add(1, { operation: 'read' });

  const readSuccess = check(readRes, {
    'READ: status is 200': (r) => r.status === 200,
    'READ: subscription found': (r) => {
      try {
        const body = JSON.parse(r.body);
        return body.subscriptionId === subscriptionId;
      } catch {
        return false;
      }
    },
  });

  if (!readSuccess) {
    crudErrors.add(1);
  }

  sleep(0.1);

  // UPDATE: Modify the subscription
  const updatePayload = {
    callback: `https://smo.example.com/notify/${resourceId}/updated`,
    filter: '(eq,resourceTypeId,compute)',
  };

  startTime = Date.now();
  const updateRes = http.put(
    `${BASE_URL}${API_VERSION}/subscriptions/${subscriptionId}`,
    JSON.stringify(updatePayload),
    params
  );
  updateLatency.add(Date.now() - startTime);
  operationCounter.add(1, { operation: 'update' });

  const updateSuccess = check(updateRes, {
    'UPDATE: status is 200': (r) => r.status === 200,
    'UPDATE: subscription updated': (r) => {
      try {
        const body = JSON.parse(r.body);
        return body.callback === updatePayload.callback;
      } catch {
        return false;
      }
    },
  });

  if (!updateSuccess) {
    crudErrors.add(1);
  }

  sleep(0.1);

  // DELETE: Remove the subscription
  startTime = Date.now();
  const deleteRes = http.del(
    `${BASE_URL}${API_VERSION}/subscriptions/${subscriptionId}`,
    null,
    params
  );
  deleteLatency.add(Date.now() - startTime);
  operationCounter.add(1, { operation: 'delete' });

  const deleteSuccess = check(deleteRes, {
    'DELETE: status is 204': (r) => r.status === 204,
  });

  if (!deleteSuccess) {
    crudErrors.add(1);
  }

  sleep(0.1);

  // VERIFY DELETE: Ensure subscription is gone
  const verifyRes = http.get(
    `${BASE_URL}${API_VERSION}/subscriptions/${subscriptionId}`,
    params
  );

  const verifySuccess = check(verifyRes, {
    'VERIFY DELETE: subscription not found': (r) => r.status === 404,
  });

  if (!verifySuccess) {
    crudErrors.add(1);
  }

  // Think time between full CRUD cycles
  sleep(2);
}

export function teardown(data) {
  console.log('CRUD operations test completed');
  console.log('Check metrics for operation latencies:');
  console.log('- CREATE: subscription creation time');
  console.log('- READ: subscription retrieval time');
  console.log('- UPDATE: subscription modification time');
  console.log('- DELETE: subscription deletion time');
}
