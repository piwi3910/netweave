# Webhook Security

**Version:** 1.0
**Last Updated:** 2026-01-14

## Overview

O2-IMS webhooks use HMAC-SHA256 signatures to ensure authenticity and integrity of event notifications. All webhook consumers (SMO implementations) MUST verify signatures before processing events.

## Signature Mechanism

### Headers

Every webhook request includes three security headers:

| Header | Description | Example |
|--------|-------------|---------|
| `X-O2IMS-Signature` | HMAC-SHA256 signature (hex-encoded) | `a3f2b9c1d4e5f6a7b8c9d0e1f2a3b4c5...` |
| `X-O2IMS-Timestamp` | Unix timestamp (seconds) | `1705244400` |
| `X-O2IMS-Event-Type` | Event type identifier | `resource.created` |

### Signature Computation

The signature is computed as:

```
signature = HMAC-SHA256(secret, timestamp + "." + body)
```

**Where:**
- `secret` - Shared secret from subscription creation
- `timestamp` - Unix timestamp from `X-O2IMS-Timestamp` header
- `body` - Raw JSON request body (no whitespace modifications)

### Verification Algorithm

Consumer implementations MUST:

1. **Extract headers**
   ```
   signature = request.headers["X-O2IMS-Signature"]
   timestamp = request.headers["X-O2IMS-Timestamp"]
   ```

2. **Verify timestamp freshness** (replay protection)
   ```
   if |current_time - timestamp| > 300 seconds:
       reject request
   ```

3. **Reconstruct signed payload**
   ```
   payload = timestamp + "." + raw_body
   ```

4. **Compute expected signature**
   ```
   expected = HMAC-SHA256(secret, payload)
   ```

5. **Compare signatures** (constant-time comparison)
   ```
   if not constant_time_compare(signature, expected):
       reject request
   ```

## Implementation Examples

### Python (Flask)

```python
import hmac
import hashlib
import time
from flask import Flask, request, jsonify

app = Flask(__name__)

def verify_webhook(request, secret):
    """
    Verify O2-IMS webhook signature.

    Args:
        request: Flask request object
        secret: Subscription secret string

    Returns:
        bool: True if signature is valid
    """
    signature = request.headers.get('X-O2IMS-Signature')
    timestamp = request.headers.get('X-O2IMS-Timestamp')

    if not signature or not timestamp:
        return False

    # Verify timestamp (5 minute window)
    try:
        ts = int(timestamp)
        if abs(time.time() - ts) > 300:
            return False
    except ValueError:
        return False

    # Get raw body
    body = request.get_data()

    # Compute expected signature
    payload = f"{timestamp}.{body.decode('utf-8')}"
    expected = hmac.new(
        secret.encode('utf-8'),
        payload.encode('utf-8'),
        hashlib.sha256
    ).hexdigest()

    # Constant-time comparison
    return hmac.compare_digest(signature, expected)

@app.route('/webhook', methods=['POST'])
def handle_webhook():
    secret = os.getenv('WEBHOOK_SECRET')

    if not verify_webhook(request, secret):
        return jsonify({'error': 'Invalid signature'}), 401

    event = request.json
    # Process event...

    return jsonify({'status': 'received'}), 200
```

### Go

```go
package main

import (
    "bytes"
    "crypto/hmac"
    "crypto/sha256"
    "encoding/hex"
    "io"
    "net/http"
    "strconv"
    "time"
)

// VerifyWebhook validates O2-IMS webhook signature
func VerifyWebhook(r *http.Request, secret string) bool {
    signature := r.Header.Get("X-O2IMS-Signature")
    timestamp := r.Header.Get("X-O2IMS-Timestamp")

    if signature == "" || timestamp == "" {
        return false
    }

    // Verify timestamp (5 minute window)
    ts, err := strconv.ParseInt(timestamp, 10, 64)
    if err != nil {
        return false
    }
    if time.Now().Unix()-ts > 300 {
        return false
    }

    // Read and restore body
    body, err := io.ReadAll(r.Body)
    if err != nil {
        return false
    }
    r.Body = io.NopCloser(bytes.NewBuffer(body))

    // Compute expected signature
    payload := timestamp + "." + string(body)
    h := hmac.New(sha256.New, []byte(secret))
    h.Write([]byte(payload))
    expected := hex.EncodeToString(h.Sum(nil))

    // Constant-time comparison
    return hmac.Equal([]byte(signature), []byte(expected))
}

func webhookHandler(w http.ResponseWriter, r *http.Request) {
    secret := os.Getenv("WEBHOOK_SECRET")

    if !VerifyWebhook(r, secret) {
        http.Error(w, "Invalid signature", http.StatusUnauthorized)
        return
    }

    // Process webhook event
    w.WriteHeader(http.StatusOK)
}
```

### Java (Spring Boot)

```java
package com.example.webhook;

import org.springframework.stereotype.Component;
import org.springframework.web.util.ContentCachingRequestWrapper;

import javax.crypto.Mac;
import javax.crypto.spec.SecretKeySpec;
import javax.servlet.http.HttpServletRequest;
import java.nio.charset.StandardCharsets;
import java.security.MessageDigest;
import java.time.Instant;

@Component
public class WebhookVerifier {

    private static final int TIMESTAMP_TOLERANCE = 300; // 5 minutes

    /**
     * Verify O2-IMS webhook signature
     *
     * @param request HTTP request (must be ContentCachingRequestWrapper)
     * @param secret Subscription secret
     * @return true if signature is valid
     */
    public boolean verify(HttpServletRequest request, String secret) {
        String signature = request.getHeader("X-O2IMS-Signature");
        String timestamp = request.getHeader("X-O2IMS-Timestamp");

        if (signature == null || timestamp == null) {
            return false;
        }

        // Verify timestamp
        try {
            long ts = Long.parseLong(timestamp);
            long now = Instant.now().getEpochSecond();
            if (Math.abs(now - ts) > TIMESTAMP_TOLERANCE) {
                return false;
            }
        } catch (NumberFormatException e) {
            return false;
        }

        // Get request body
        ContentCachingRequestWrapper wrapper =
            (ContentCachingRequestWrapper) request;
        byte[] body = wrapper.getContentAsByteArray();
        String bodyStr = new String(body, StandardCharsets.UTF_8);

        // Compute signature
        try {
            String payload = timestamp + "." + bodyStr;
            Mac mac = Mac.getInstance("HmacSHA256");
            SecretKeySpec keySpec = new SecretKeySpec(
                secret.getBytes(StandardCharsets.UTF_8),
                "HmacSHA256"
            );
            mac.init(keySpec);
            byte[] hash = mac.doFinal(payload.getBytes(StandardCharsets.UTF_8));

            // Convert to hex string
            StringBuilder expected = new StringBuilder();
            for (byte b : hash) {
                expected.append(String.format("%02x", b));
            }

            // Constant-time comparison
            return MessageDigest.isEqual(
                signature.getBytes(StandardCharsets.UTF_8),
                expected.toString().getBytes(StandardCharsets.UTF_8)
            );
        } catch (Exception e) {
            return false;
        }
    }
}

// Controller example
@RestController
@RequestMapping("/webhook")
public class WebhookController {

    @Autowired
    private WebhookVerifier verifier;

    @Value("${webhook.secret}")
    private String secret;

    @PostMapping
    public ResponseEntity<Map<String, String>> handleWebhook(
            HttpServletRequest request,
            @RequestBody Map<String, Object> event) {

        if (!verifier.verify(request, secret)) {
            return ResponseEntity.status(HttpStatus.UNAUTHORIZED)
                .body(Map.of("error", "Invalid signature"));
        }

        // Process event...

        return ResponseEntity.ok(Map.of("status", "received"));
    }
}
```

### Node.js (Express)

```javascript
const express = require('express');
const crypto = require('crypto');

const app = express();

// Use raw body parser for signature verification
app.use('/webhook', express.raw({ type: 'application/json' }));

function verifyWebhook(req, secret) {
    const signature = req.headers['x-o2ims-signature'];
    const timestamp = req.headers['x-o2ims-timestamp'];

    if (!signature || !timestamp) {
        return false;
    }

    // Verify timestamp (5 minute window)
    const now = Math.floor(Date.now() / 1000);
    const ts = parseInt(timestamp, 10);
    if (Math.abs(now - ts) > 300) {
        return false;
    }

    // Compute expected signature
    const payload = `${timestamp}.${req.body.toString('utf8')}`;
    const expected = crypto
        .createHmac('sha256', secret)
        .update(payload)
        .digest('hex');

    // Constant-time comparison
    return crypto.timingSafeEqual(
        Buffer.from(signature),
        Buffer.from(expected)
    );
}

app.post('/webhook', (req, res) => {
    const secret = process.env.WEBHOOK_SECRET;

    if (!verifyWebhook(req, secret)) {
        return res.status(401).json({ error: 'Invalid signature' });
    }

    // Parse and process event
    const event = JSON.parse(req.body.toString('utf8'));

    // Process event...

    res.json({ status: 'received' });
});
```

## Security Best Practices

### 1. Secret Management

**DO:**
- ✅ Store secrets in environment variables or secret management systems
- ✅ Rotate secrets every 90 days
- ✅ Use different secrets for production and non-production environments
- ✅ Generate secrets with cryptographically secure random number generators

**DON'T:**
- ❌ Hardcode secrets in source code
- ❌ Log secrets or include in error messages
- ❌ Share secrets via email or insecure channels
- ❌ Reuse secrets across multiple subscriptions

### 2. Timestamp Validation

**Always verify timestamp freshness** to prevent replay attacks:

```python
MAX_AGE = 300  # 5 minutes

def is_timestamp_valid(timestamp):
    try:
        ts = int(timestamp)
        age = abs(time.time() - ts)
        return age <= MAX_AGE
    except (ValueError, TypeError):
        return False
```

**Recommended window:** 5 minutes (300 seconds)

### 3. Constant-Time Comparison

**NEVER use `==` for signature comparison** - it's vulnerable to timing attacks.

**Use constant-time comparison functions:**

| Language | Function |
|----------|----------|
| Python | `hmac.compare_digest(a, b)` |
| Go | `hmac.Equal([]byte(a), []byte(b))` |
| Java | `MessageDigest.isEqual(a, b)` |
| Node.js | `crypto.timingSafeEqual(a, b)` |
| Ruby | `Rack::Utils.secure_compare(a, b)` |
| PHP | `hash_equals($a, $b)` |

### 4. HTTPS Only

**Production endpoints MUST use HTTPS:**

```yaml
# Subscription creation
{
  "callback": "https://smo.example.com/webhook",  # ✅ HTTPS
  "filter": "..."
}
```

**Never accept webhooks over HTTP in production:**

```python
@app.before_request
def enforce_https():
    if not request.is_secure and not app.debug:
        abort(403, "HTTPS required")
```

### 5. Error Handling

**DO NOT leak verification failures:**

```python
# ❌ BAD - Reveals verification details
if not signature:
    return {'error': 'Missing signature'}, 401
if not timestamp_valid:
    return {'error': 'Timestamp expired'}, 401
if not signature_valid:
    return {'error': 'Signature mismatch'}, 401

# ✅ GOOD - Generic error
if not verify_webhook(request, secret):
    return {'error': 'Unauthorized'}, 401
```

### 6. Body Handling

**Critical:** Signature is computed over the **raw** request body.

```python
# ✅ GOOD - Use raw body
body = request.get_data()  # Raw bytes
payload = f"{timestamp}.{body.decode('utf-8')}"

# ❌ BAD - Don't re-serialize JSON
body = json.dumps(request.json)  # Wrong! Whitespace may differ
```

## Testing

### Manual Testing Script

```bash
#!/bin/bash
# test-webhook.sh - Test webhook signature verification

set -e

SECRET="${WEBHOOK_SECRET:-test-secret}"
ENDPOINT="${WEBHOOK_ENDPOINT:-http://localhost:8080/webhook}"
TIMESTAMP=$(date +%s)
BODY='{"eventType":"resource.created","resourceId":"res-123"}'

# Compute signature
PAYLOAD="${TIMESTAMP}.${BODY}"
SIGNATURE=$(echo -n "$PAYLOAD" | openssl dgst -sha256 -hmac "$SECRET" | cut -d' ' -f2)

echo "Testing webhook endpoint: $ENDPOINT"
echo "Timestamp: $TIMESTAMP"
echo "Signature: $SIGNATURE"
echo ""

# Send request
curl -X POST "$ENDPOINT" \
  -H "Content-Type: application/json" \
  -H "X-O2IMS-Signature: $SIGNATURE" \
  -H "X-O2IMS-Timestamp: $TIMESTAMP" \
  -H "X-O2IMS-Event-Type: resource.created" \
  -d "$BODY" \
  -v

echo ""
echo "✅ Test complete"
```

### Unit Test Example (Python)

```python
import unittest
import time
import hmac
import hashlib

class TestWebhookVerification(unittest.TestCase):

    def setUp(self):
        self.secret = "test-secret-key"
        self.timestamp = str(int(time.time()))
        self.body = '{"eventType":"test"}'

    def compute_signature(self, timestamp, body):
        payload = f"{timestamp}.{body}"
        return hmac.new(
            self.secret.encode(),
            payload.encode(),
            hashlib.sha256
        ).hexdigest()

    def test_valid_signature(self):
        signature = self.compute_signature(self.timestamp, self.body)

        # Simulate request
        request = MockRequest(
            headers={
                'X-O2IMS-Signature': signature,
                'X-O2IMS-Timestamp': self.timestamp
            },
            body=self.body.encode()
        )

        self.assertTrue(verify_webhook(request, self.secret))

    def test_invalid_signature(self):
        request = MockRequest(
            headers={
                'X-O2IMS-Signature': 'invalid',
                'X-O2IMS-Timestamp': self.timestamp
            },
            body=self.body.encode()
        )

        self.assertFalse(verify_webhook(request, self.secret))

    def test_expired_timestamp(self):
        old_timestamp = str(int(time.time()) - 400)  # 400 seconds ago
        signature = self.compute_signature(old_timestamp, self.body)

        request = MockRequest(
            headers={
                'X-O2IMS-Signature': signature,
                'X-O2IMS-Timestamp': old_timestamp
            },
            body=self.body.encode()
        )

        self.assertFalse(verify_webhook(request, self.secret))
```

## Subscription API Response

When creating a subscription, the API returns security configuration:

```json
POST /o2ims-infrastructureInventory/v1/subscriptions

Response:
{
  "subscriptionId": "sub-a1b2c3d4",
  "callback": "https://smo.example.com/webhook",
  "filter": "resource.created;resource.deleted",
  "security": {
    "type": "hmac-sha256",
    "secret": "7d3e9f2a8c1b4e6d5f8a9c2b7e1d4f3a",
    "headers": {
      "signature": "X-O2IMS-Signature",
      "timestamp": "X-O2IMS-Timestamp",
      "eventType": "X-O2IMS-Event-Type"
    },
    "algorithm": "HMAC-SHA256",
    "encoding": "hex",
    "payload_format": "{timestamp}.{body}",
    "timestamp_tolerance": 300,
    "documentation": "https://docs.netweave.io/webhook-security"
  },
  "consumerSubscriptionId": "smo-sub-789",
  "createdAt": "2026-01-14T10:30:00Z"
}
```

**IMPORTANT:** Store the `security.secret` value securely. It cannot be retrieved later.

## Troubleshooting

### Common Issues

#### 1. Signature Mismatch

**Symptom:** Signature verification always fails

**Causes:**
- Body modification (whitespace, encoding)
- Incorrect payload format
- Wrong secret
- Timestamp/body order reversed

**Debug:**
```python
# Log the exact payload used
payload = f"{timestamp}.{body}"
print(f"Payload: {repr(payload)}")
print(f"Expected signature: {expected}")
print(f"Received signature: {signature}")
```

#### 2. Timestamp Expired

**Symptom:** "Timestamp expired" errors

**Causes:**
- Server clock skew
- Network delays
- Too strict tolerance window

**Solutions:**
- Sync server clocks with NTP
- Increase tolerance to 5 minutes
- Log time differences for debugging

#### 3. Missing Headers

**Symptom:** Headers are None/null

**Causes:**
- Proxy stripping headers
- Case-sensitivity issues
- Framework middleware interference

**Solutions:**
- Check proxy configuration
- Use case-insensitive header lookup
- Verify middleware order

## References

### Specifications

- [RFC 2104: HMAC](https://www.rfc-editor.org/rfc/rfc2104)
- [RFC 6238: TOTP (Time-based concepts)](https://www.rfc-editor.org/rfc/rfc6238)
- [O-RAN O2 IMS Specification](https://specifications.o-ran.org/)

### Security Resources

- [OWASP API Security Top 10](https://owasp.org/API-Security/)
- [Webhook Security Best Practices](https://webhooks.fyi/security/hmac)
- [Timing Attack Vulnerabilities](https://codahale.com/a-lesson-in-timing-attacks/)

### Implementation Guides

- [Stripe Webhook Security](https://stripe.com/docs/webhooks/signatures)
- [GitHub Webhook Security](https://docs.github.com/webhooks/securing)
- [Twilio Webhook Security](https://www.twilio.com/docs/usage/security)

## Support

For issues or questions:
- GitHub Issues: https://github.com/piwi3910/netweave/issues
- Documentation: https://docs.netweave.io
- Security issues: security@netweave.io (private disclosure)
