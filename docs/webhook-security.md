# Webhook Security Guide

This document describes how to securely verify webhook notifications from the O2-IMS Gateway using HMAC-SHA256 signature verification.

## Overview

The O2-IMS Gateway uses HMAC-SHA256 signatures to ensure webhook notifications are:
1. **Authentic** - Originated from the O2-IMS Gateway
2. **Unmodified** - Payload has not been tampered with in transit

## Webhook Headers

Every webhook notification includes the following security-related headers:

| Header | Description |
|--------|-------------|
| `X-O2IMS-Signature` | HMAC-SHA256 signature of the request body |
| `X-O2IMS-Timestamp` | Unix timestamp when the notification was sent |
| `X-O2IMS-Event-Type` | Type of event (e.g., `ResourceCreated`, `ResourceModified`) |
| `X-O2IMS-Notification-ID` | Unique identifier for this notification |
| `X-O2IMS-Subscription-ID` | ID of the subscription that triggered this notification |

## Signature Verification Algorithm

### Step 1: Extract Required Headers

Extract the signature and timestamp from the request headers:

```
signature = headers["X-O2IMS-Signature"]
timestamp = headers["X-O2IMS-Timestamp"]
```

### Step 2: Validate Timestamp (Replay Protection)

Reject requests with timestamps older than 5 minutes to prevent replay attacks:

```
current_time = current_unix_timestamp()
if abs(current_time - timestamp) > 300:
    reject_request("timestamp too old")
```

### Step 3: Reconstruct Signed Payload

The signed payload is constructed as: `{timestamp}.{request_body}`

```
signed_payload = timestamp + "." + raw_request_body
```

### Step 4: Compute Expected Signature

Compute HMAC-SHA256 of the signed payload using your webhook secret:

```
expected_signature = HMAC-SHA256(signed_payload, webhook_secret)
```

### Step 5: Compare Signatures

Use constant-time comparison to prevent timing attacks:

```
if constant_time_compare(signature, expected_signature):
    accept_request()
else:
    reject_request("invalid signature")
```

## Implementation Examples

### Python (Flask)

```python
import hmac
import hashlib
import time
from functools import wraps
from flask import Flask, request, jsonify

app = Flask(__name__)
WEBHOOK_SECRET = "your-webhook-secret"
TIMESTAMP_TOLERANCE = 300  # 5 minutes

def verify_webhook_signature(f):
    @wraps(f)
    def decorated(*args, **kwargs):
        # Extract headers
        signature = request.headers.get('X-O2IMS-Signature', '')
        timestamp = request.headers.get('X-O2IMS-Timestamp', '')

        if not signature or not timestamp:
            return jsonify({"error": "Missing signature headers"}), 401

        # Validate timestamp
        try:
            ts = int(timestamp)
            if abs(time.time() - ts) > TIMESTAMP_TOLERANCE:
                return jsonify({"error": "Request timestamp too old"}), 401
        except ValueError:
            return jsonify({"error": "Invalid timestamp"}), 401

        # Reconstruct signed payload
        signed_payload = f"{timestamp}.{request.data.decode('utf-8')}"

        # Compute expected signature
        expected = hmac.new(
            WEBHOOK_SECRET.encode('utf-8'),
            signed_payload.encode('utf-8'),
            hashlib.sha256
        ).hexdigest()

        # Constant-time comparison
        if not hmac.compare_digest(signature, expected):
            return jsonify({"error": "Invalid signature"}), 401

        return f(*args, **kwargs)
    return decorated

@app.route('/webhook', methods=['POST'])
@verify_webhook_signature
def handle_webhook():
    event = request.json
    print(f"Received event: {event['eventType']}")
    return jsonify({"status": "ok"})

if __name__ == '__main__':
    app.run(port=8080, ssl_context='adhoc')
```

### Go

```go
package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"log"
	"math"
	"net/http"
	"strconv"
	"time"
)

const (
	webhookSecret      = "your-webhook-secret"
	timestampTolerance = 300 // 5 minutes
)

func verifySignature(r *http.Request, body []byte) bool {
	signature := r.Header.Get("X-O2IMS-Signature")
	timestamp := r.Header.Get("X-O2IMS-Timestamp")

	if signature == "" || timestamp == "" {
		return false
	}

	// Validate timestamp
	ts, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil {
		return false
	}
	if math.Abs(float64(time.Now().Unix()-ts)) > timestampTolerance {
		return false
	}

	// Reconstruct signed payload
	signedPayload := timestamp + "." + string(body)

	// Compute expected signature
	mac := hmac.New(sha256.New, []byte(webhookSecret))
	mac.Write([]byte(signedPayload))
	expected := hex.EncodeToString(mac.Sum(nil))

	// Constant-time comparison
	return hmac.Equal([]byte(signature), []byte(expected))
}

func webhookHandler(w http.ResponseWriter, r *http.Request) {
	// Read body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read body", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	// Verify signature
	if !verifySignature(r, body) {
		http.Error(w, "Invalid signature", http.StatusUnauthorized)
		return
	}

	log.Printf("Received verified webhook: %s", string(body))
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status": "ok"}`))
}

func main() {
	http.HandleFunc("/webhook", webhookHandler)
	log.Fatal(http.ListenAndServeTLS(":8080", "cert.pem", "key.pem", nil))
}
```

### Java (Spring Boot)

```java
package com.example.webhook;

import org.springframework.http.HttpStatus;
import org.springframework.http.ResponseEntity;
import org.springframework.web.bind.annotation.*;

import javax.crypto.Mac;
import javax.crypto.spec.SecretKeySpec;
import java.nio.charset.StandardCharsets;
import java.security.InvalidKeyException;
import java.security.MessageDigest;
import java.security.NoSuchAlgorithmException;
import java.time.Instant;

@RestController
public class WebhookController {

    private static final String WEBHOOK_SECRET = "your-webhook-secret";
    private static final long TIMESTAMP_TOLERANCE = 300; // 5 minutes

    @PostMapping("/webhook")
    public ResponseEntity<String> handleWebhook(
            @RequestHeader("X-O2IMS-Signature") String signature,
            @RequestHeader("X-O2IMS-Timestamp") String timestamp,
            @RequestBody String body) {

        // Validate timestamp
        long ts = Long.parseLong(timestamp);
        long now = Instant.now().getEpochSecond();
        if (Math.abs(now - ts) > TIMESTAMP_TOLERANCE) {
            return ResponseEntity.status(HttpStatus.UNAUTHORIZED)
                    .body("{\"error\": \"Request timestamp too old\"}");
        }

        // Reconstruct signed payload
        String signedPayload = timestamp + "." + body;

        // Compute expected signature
        String expected = computeHmacSha256(signedPayload, WEBHOOK_SECRET);

        // Constant-time comparison
        if (!MessageDigest.isEqual(
                signature.getBytes(StandardCharsets.UTF_8),
                expected.getBytes(StandardCharsets.UTF_8))) {
            return ResponseEntity.status(HttpStatus.UNAUTHORIZED)
                    .body("{\"error\": \"Invalid signature\"}");
        }

        System.out.println("Received verified webhook: " + body);
        return ResponseEntity.ok("{\"status\": \"ok\"}");
    }

    private String computeHmacSha256(String data, String secret) {
        try {
            Mac mac = Mac.getInstance("HmacSHA256");
            SecretKeySpec secretKey = new SecretKeySpec(
                    secret.getBytes(StandardCharsets.UTF_8), "HmacSHA256");
            mac.init(secretKey);
            byte[] hash = mac.doFinal(data.getBytes(StandardCharsets.UTF_8));
            return bytesToHex(hash);
        } catch (NoSuchAlgorithmException | InvalidKeyException e) {
            throw new RuntimeException("Failed to compute HMAC", e);
        }
    }

    private static String bytesToHex(byte[] bytes) {
        StringBuilder sb = new StringBuilder();
        for (byte b : bytes) {
            sb.append(String.format("%02x", b));
        }
        return sb.toString();
    }
}
```

### Node.js (Express)

```javascript
const express = require('express');
const crypto = require('crypto');

const app = express();
const WEBHOOK_SECRET = 'your-webhook-secret';
const TIMESTAMP_TOLERANCE = 300; // 5 minutes

// Use raw body parser for signature verification
app.use('/webhook', express.raw({ type: 'application/json' }));

function verifySignature(req, res, next) {
    const signature = req.headers['x-o2ims-signature'];
    const timestamp = req.headers['x-o2ims-timestamp'];

    if (!signature || !timestamp) {
        return res.status(401).json({ error: 'Missing signature headers' });
    }

    // Validate timestamp
    const ts = parseInt(timestamp, 10);
    const now = Math.floor(Date.now() / 1000);
    if (Math.abs(now - ts) > TIMESTAMP_TOLERANCE) {
        return res.status(401).json({ error: 'Request timestamp too old' });
    }

    // Reconstruct signed payload
    const signedPayload = `${timestamp}.${req.body.toString()}`;

    // Compute expected signature
    const expected = crypto
        .createHmac('sha256', WEBHOOK_SECRET)
        .update(signedPayload)
        .digest('hex');

    // Constant-time comparison
    if (!crypto.timingSafeEqual(Buffer.from(signature), Buffer.from(expected))) {
        return res.status(401).json({ error: 'Invalid signature' });
    }

    // Parse JSON for downstream handlers
    req.body = JSON.parse(req.body.toString());
    next();
}

app.post('/webhook', verifySignature, (req, res) => {
    console.log('Received verified webhook:', req.body);
    res.json({ status: 'ok' });
});

app.listen(8080, () => {
    console.log('Webhook server listening on port 8080');
});
```

## Security Best Practices

### 1. Secret Rotation

Rotate your webhook secrets regularly (recommended: every 90 days):

```yaml
# Example rotation schedule
secrets:
  current: "secret-2024-q1"
  previous: "secret-2023-q4"  # Accept during grace period
  rotation_interval: 90d
  grace_period: 7d
```

### 2. Replay Protection

Always validate the timestamp to prevent replay attacks:
- Reject requests with timestamps older than 5 minutes
- Store processed notification IDs to detect duplicates

```python
processed_notifications = set()  # Use Redis in production

def is_duplicate(notification_id):
    if notification_id in processed_notifications:
        return True
    processed_notifications.add(notification_id)
    return False
```

### 3. Constant-Time Comparison

Always use constant-time comparison functions to prevent timing attacks:

| Language | Function |
|----------|----------|
| Python | `hmac.compare_digest()` |
| Go | `hmac.Equal()` |
| Java | `MessageDigest.isEqual()` |
| Node.js | `crypto.timingSafeEqual()` |

### 4. HTTPS Enforcement

Always use HTTPS for webhook endpoints:
- Never accept webhooks over plain HTTP in production
- Configure TLS 1.2+ with strong cipher suites
- Use valid certificates from trusted CAs

### 5. Error Handling

Never leak sensitive information in error responses:

```python
# Bad - reveals internal details
return {"error": f"HMAC mismatch: expected {expected}, got {signature}"}

# Good - generic error message
return {"error": "Invalid signature"}
```

### 6. Secret Storage

Store webhook secrets securely:
- Use environment variables or secret management systems
- Never commit secrets to version control
- Encrypt secrets at rest

```yaml
# Kubernetes Secret example
apiVersion: v1
kind: Secret
metadata:
  name: webhook-secret
type: Opaque
data:
  secret: <base64-encoded-secret>
```

## Testing Webhook Verification

Use this bash script to generate test webhooks with valid signatures:

```bash
#!/bin/bash
# test-webhook.sh - Test webhook signature generation

WEBHOOK_SECRET="${WEBHOOK_SECRET:-your-webhook-secret}"
WEBHOOK_URL="${WEBHOOK_URL:-https://your-smo.example.com/webhook}"
TIMESTAMP=$(date +%s)

# Sample event payload
PAYLOAD=$(cat <<EOF
{
  "subscriptionId": "sub-123",
  "eventType": "ResourceCreated",
  "resource": {
    "resourceId": "res-456",
    "resourceTypeId": "compute"
  }
}
EOF
)

# Construct signed payload (timestamp.body)
SIGNED_PAYLOAD="${TIMESTAMP}.${PAYLOAD}"

# Generate HMAC-SHA256 signature
SIGNATURE=$(echo -n "$SIGNED_PAYLOAD" | openssl dgst -sha256 -hmac "$WEBHOOK_SECRET" | awk '{print $2}')

echo "Sending webhook to: $WEBHOOK_URL"
echo "Timestamp: $TIMESTAMP"
echo "Signature: $SIGNATURE"

# Send webhook
curl -v -X POST "$WEBHOOK_URL" \
  -H "Content-Type: application/json" \
  -H "X-O2IMS-Signature: $SIGNATURE" \
  -H "X-O2IMS-Timestamp: $TIMESTAMP" \
  -H "X-O2IMS-Event-Type: ResourceCreated" \
  -H "X-O2IMS-Notification-ID: test-$(uuidgen)" \
  -H "X-O2IMS-Subscription-ID: sub-123" \
  -d "$PAYLOAD"
```

## Subscription Creation Response

When you create a subscription, the response includes security metadata:

```json
{
  "subscriptionId": "sub-12345",
  "callback": "https://your-smo.example.com/webhook",
  "filter": "...",
  "security": {
    "authenticationType": "HMAC-SHA256",
    "secretReference": "webhook-secrets/sub-12345",
    "headers": [
      "X-O2IMS-Signature",
      "X-O2IMS-Timestamp",
      "X-O2IMS-Event-Type",
      "X-O2IMS-Notification-ID",
      "X-O2IMS-Subscription-ID"
    ],
    "documentationUrl": "https://docs.example.com/webhook-security"
  }
}
```

## Troubleshooting

### Common Issues

1. **"Invalid signature" errors**
   - Verify the secret matches on both ends
   - Ensure body is not modified by middleware before verification
   - Check that timestamp format is correct (Unix seconds, not milliseconds)

2. **"Timestamp too old" errors**
   - Verify server clocks are synchronized (use NTP)
   - Increase tolerance if network latency is high

3. **Signature mismatch**
   - Verify payload encoding (UTF-8)
   - Check for whitespace differences
   - Ensure consistent newline handling

### Debug Logging

Enable debug logging to troubleshoot signature issues:

```python
import logging
logging.basicConfig(level=logging.DEBUG)

# In your verification function:
logger.debug(f"Received signature: {signature}")
logger.debug(f"Timestamp: {timestamp}")
logger.debug(f"Raw body: {repr(body)}")
logger.debug(f"Signed payload: {repr(signed_payload)}")
logger.debug(f"Expected signature: {expected}")
```

## References

- [Webhook Best Practices](https://webhooks.fyi/security/hmac)
- [OWASP API Security Guidelines](https://owasp.org/www-project-api-security/)
- [O-RAN O2 IMS Specification](https://specifications.o-ran.org/)
