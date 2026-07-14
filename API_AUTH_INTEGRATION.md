# Radar API Auth Integration

This document describes how to access Radar API endpoints protected by API key authentication.

## Base URL

Use the API service address provided by the Radar operator.

Example:

```text
https://api.example.com
```

All business endpoints are under:

```text
/v1
```

## Authentication

For every `/v1/*` request, include one valid API key in one of the following headers.

Recommended:

```http
X-API-Key: radar_8gT4kK9vQm2Xz7PpN6cY3bF1sH5dRwL0
```

Also supported:

```http
Authorization: Bearer radar_8gT4kK9vQm2Xz7PpN6cY3bF1sH5dRwL0
```

Do not put the API key in query parameters.

## IP Whitelist

The Radar operator may whitelist specific client IPs or CIDR ranges.

Whitelisted IPs can access `/v1/*` without sending an API key. Non-whitelisted IPs must send a valid API key.

When the service is behind a proxy or load balancer, the client IP is resolved in this order:

1. `X-Real-IP`
2. First IP in `X-Forwarded-For`
3. TCP remote address

If you use a proxy, make sure it forwards the real client IP correctly.

## Error Response

If authentication fails, the API returns HTTP `401`:

```json
{
  "code": 401,
  "message": "unauthorized"
}
```

## cURL Examples

Request with `X-API-Key`:

```bash
curl 'https://api.example.com/v1/mos/max/id?project_id=1&topic=example_topic' \
  -H 'X-API-Key: radar_8gT4kK9vQm2Xz7PpN6cY3bF1sH5dRwL0'
```

Request with bearer token:

```bash
curl 'https://api.example.com/v1/mos/list?project_id=1&topic=example_topic&limit=10' \
  -H 'Authorization: Bearer radar_8gT4kK9vQm2Xz7PpN6cY3bF1sH5dRwL0'
```

## JavaScript Example

```js
const res = await fetch("https://api.example.com/v1/mos/max/id?project_id=1&topic=example_topic", {
  headers: {
    "X-API-Key": "radar_8gT4kK9vQm2Xz7PpN6cY3bF1sH5dRwL0",
  },
});

const body = await res.json();
console.log(body);
```

## Operator Configuration

The API service configuration supports:

```json
{
  "listen": ":8080",
  "dsn": "user:password@tcp(127.0.0.1:3306)/radar?charset=utf8mb4&parseTime=True&loc=Local",
  "api_auth_keys": [
    "radar_8gT4kK9vQm2Xz7PpN6cY3bF1sH5dRwL0",
    "radar_client_b_3aQ9nM2vP6xT8sLd"
  ],
  "ip_whitelist": ["127.0.0.1", "10.0.0.0/8"]
}
```

Fields:

- `api_auth_keys`: API keys accepted by the service. Use separate keys for different clients when possible.
- `ip_whitelist`: IPs or CIDR ranges that can bypass API key authentication.

## Security Notes

- Keep the API key secret.
- Rotate the API key if it is exposed.
- Remove a client's key from `api_auth_keys` when that client should no longer have access.
- Prefer HTTPS in production.
- Avoid logging request headers that contain the API key.
