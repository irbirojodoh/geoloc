# Geocode API

Endpoints for reverse geocoding (coordinates to address).

## Get Address

Get address details from GPS coordinates. Uses cached data when available, falls back to Nominatim.

**Endpoint:** `GET /api/v1/geocode/address`

> ⚠️ **Requires Authentication**

### Query Parameters

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `lat` | float | Yes | Latitude (-90 to 90) |
| `lng` | float | Yes | Longitude (-180 to 180) |

### Response

```json
{
  "geohash": "qqggy",
  "location_name": "Kukusan",
  "address": {
    "village": "Kukusan",
    "city_district": "Beji",
    "city": "Depok",
    "state": "West Java",
    "region": "",
    "postcode": "16425",
    "country": "Indonesia",
    "country_code": "id"
  }
}
```

### Example

```bash
curl "http://localhost:8080/api/v1/geocode/address?lat=-6.3694&lng=106.8246" \
  -H "Authorization: Bearer <token>"
```

### How It Works

1. Calculates 5-character geohash prefix from coordinates
2. Checks `location_names` table for cached data
3. If not cached, queries Nominatim API (rate limited)
4. Caches result for future requests
5. Returns address details

### Errors

| Status | Meaning |
|--------|---------|
| 400 | Missing or invalid lat/lng |
| 401 | Not authenticated |
| 500 | Failed to fetch address (Nominatim error) |
