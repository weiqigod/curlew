# Get user

## Notes

<!-- BEGIN apitest:response id=req-1 slug=get-user run=0123456789abcdef0123456789abcdef -->
## Response (deterministic)

### Request

GET https://api.example.com/users/1

### Response 200

```json
{
  "id": 1,
  "name": "Alice"
}
```

### Response metadata

Content-Type: application/json

### Timing

duration_ms: 42
wave_index: sequential
started_at: 2026-04-25T12:00:00Z

### Assertions

- [x] status expected="200" actual="200"
<!-- END apitest:response id=req-1 slug=get-user run=0123456789abcdef0123456789abcdef -->

## Analysis

