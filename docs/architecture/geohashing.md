# Geohashing

Geoloc uses geohashing for efficient location-based queries.

## What is a Geohash?

A geohash is a string that encodes a geographic location into a compact format. Longer geohashes represent more precise locations.

```
-6.3694, 106.8246 → "qqggy4b8r" (full precision)
-6.3694, 106.8246 → "qqggy"     (5-char, ~5km area)
```

## Geohash Precision

| Characters | Cell Size | Use Case |
|------------|-----------|----------|
| 1 | ~5000 km | Continent |
| 2 | ~1250 km | Large country |
| 3 | ~156 km | State/region |
| 4 | ~39 km | City |
| **5** | **~5 km** | **Neighborhood (Geoloc default)** |
| 6 | ~1.2 km | Street |
| 7 | ~150 m | Building |
| 8 | ~38 m | Address |

Geoloc uses **5-character geohash prefixes** (~5km cells) for feed queries.

## How Location Queries Work

### Problem
Finding all posts within 5km of a location would require scanning every post—inefficient at scale.

### Solution
1. Convert query location to 5-char geohash prefix
2. Get 8 neighboring geohash cells (9 total including center)
3. Query only those 9 partitions
4. Apply precise distance filtering on results

```
┌─────┬─────┬─────┐
│ NW  │  N  │ NE  │
├─────┼─────┼─────┤
│  W  │  *  │  E  │  * = Query location
├─────┼─────┼─────┤
│ SW  │  S  │ SE  │
└─────┴─────┴─────┘
```

### Code Example

```go
// Get center geohash and 8 neighbors
neighbors := data.GetNeighbors(latitude, longitude)
// Returns: ["qqggy", "qqggz", "qqggx", "qqggt", ...]

// Query each partition
for _, geohashPrefix := range neighbors {
    posts := queryPostsByGeohash(geohashPrefix)
    // Filter by actual distance
    for _, post := range posts {
        distance := HaversineDistance(lat, lng, post.Lat, post.Lng)
        if distance <= radiusKM {
            results = append(results, post)
        }
    }
}
```

## Haversine Distance

Precise distance between two coordinates on Earth's surface:

```go
func HaversineDistance(lat1, lng1, lat2, lng2 float64) float64 {
    const R = 6371.0 // Earth radius in km
    
    dLat := (lat2 - lat1) * math.Pi / 180
    dLng := (lng2 - lng1) * math.Pi / 180
    
    a := math.Sin(dLat/2)*math.Sin(dLat/2) +
         math.Cos(lat1*math.Pi/180)*math.Cos(lat2*math.Pi/180)*
         math.Sin(dLng/2)*math.Sin(dLng/2)
    
    c := 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
    
    return R * c
}
```

## Privacy

Posts return only the 5-character geohash prefix, not precise coordinates. This provides:
- ~5km location accuracy (neighborhood level)
- Privacy protection for users
- Enough precision for "nearby" context

## Libraries Used

- [mmcloughlin/geohash](https://github.com/mmcloughlin/geohash) - Go geohash encoding
