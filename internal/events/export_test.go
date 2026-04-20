package events

// This file exposes internal helpers to the external-package test files
// (events_test) via compile-time indirection. The exports only exist under
// `go test`; they are not part of the public API.

// SubscriptionBucketForTest exposes subscriptionBucket for the external test package.
// It returns the deterministic 16-bit bucket identifier (4-char lowercase hex)
// used to bound the cardinality of the notifications_delivered_total label space.
func SubscriptionBucketForTest(subscriptionID string) string {
	return subscriptionBucket(subscriptionID)
}

// CallbackHostForTest exposes callbackHost for the external test package.
// It returns the host portion of a callback URL, or a stable fallback when
// the URL is empty or unparseable.
func CallbackHostForTest(callbackURL string) string {
	return callbackHost(callbackURL)
}
