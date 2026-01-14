#!/bin/bash
# test-webhook.sh - Test O2-IMS webhook signature verification
#
# Usage:
#   ./test-webhook.sh [endpoint_url] [secret]
#
# Environment Variables:
#   WEBHOOK_SECRET   - Shared secret from subscription creation (default: "test-secret")
#   WEBHOOK_ENDPOINT - Target webhook endpoint (default: http://localhost:8080/webhook)
#
# Examples:
#   ./test-webhook.sh
#   ./test-webhook.sh https://smo.example.com/webhook my-secret-key
#   WEBHOOK_SECRET=prod-secret ./test-webhook.sh https://prod.example.com/webhook

set -e

# Configuration
SECRET="${2:-${WEBHOOK_SECRET:-test-secret}}"
ENDPOINT="${1:-${WEBHOOK_ENDPOINT:-http://localhost:8080/webhook}}"
TIMESTAMP=$(date +%s)

# Event payload
BODY='{
  "eventType": "resource.created",
  "eventId": "evt-'$(uuidgen | tr '[:upper:]' '[:lower:]')'",
  "timestamp": "'$(date -u +"%Y-%m-%dT%H:%M:%SZ")'",
  "resourcePoolId": "pool-test-001",
  "resourceId": "res-'$(uuidgen | tr '[:upper:]' '[:lower:]')'",
  "data": {
    "name": "test-resource",
    "status": "active",
    "type": "compute-node"
  }
}'

# Compute HMAC-SHA256 signature
PAYLOAD="${TIMESTAMP}.${BODY}"
SIGNATURE=$(echo -n "$PAYLOAD" | openssl dgst -sha256 -hmac "$SECRET" | cut -d' ' -f2)

# Display test information
echo "=========================================="
echo "O2-IMS Webhook Security Test"
echo "=========================================="
echo ""
echo "Endpoint:  $ENDPOINT"
echo "Secret:    ${SECRET:0:8}... (${#SECRET} chars)"
echo "Timestamp: $TIMESTAMP"
echo "Signature: ${SIGNATURE:0:16}..."
echo ""
echo "Headers:"
echo "  X-O2IMS-Signature: $SIGNATURE"
echo "  X-O2IMS-Timestamp: $TIMESTAMP"
echo "  X-O2IMS-Event-Type: resource.created"
echo ""
echo "Payload (first 200 chars):"
echo "${BODY:0:200}..."
echo ""
echo "=========================================="
echo "Sending request..."
echo "=========================================="
echo ""

# Send webhook request
HTTP_CODE=$(curl -X POST "$ENDPOINT" \
  -H "Content-Type: application/json" \
  -H "X-O2IMS-Signature: $SIGNATURE" \
  -H "X-O2IMS-Timestamp: $TIMESTAMP" \
  -H "X-O2IMS-Event-Type: resource.created" \
  -d "$BODY" \
  -w "%{http_code}" \
  -o /tmp/webhook-response.txt \
  -s)

# Display response
echo "HTTP Status: $HTTP_CODE"
echo ""
echo "Response Body:"
cat /tmp/webhook-response.txt
echo ""
echo ""

# Interpret result
if [ "$HTTP_CODE" = "200" ] || [ "$HTTP_CODE" = "201" ] || [ "$HTTP_CODE" = "202" ]; then
  echo "✅ SUCCESS - Webhook accepted and signature verified"
  exit 0
elif [ "$HTTP_CODE" = "401" ] || [ "$HTTP_CODE" = "403" ]; then
  echo "❌ FAILURE - Authentication failed (check secret and signature)"
  exit 1
elif [ "$HTTP_CODE" = "000" ]; then
  echo "❌ FAILURE - Cannot connect to endpoint (check URL and network)"
  exit 1
else
  echo "⚠️  WARNING - Unexpected HTTP status code: $HTTP_CODE"
  exit 1
fi
