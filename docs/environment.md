# Environment Configuration

All configuration is done via environment variables. Copy `.env.example` to `.env` and customize as needed.

## Required Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `CASSANDRA_HOST` | Cassandra host address | `localhost` |
| `CASSANDRA_KEYSPACE` | Keyspace name | `geoloc` |
| `JWT_SECRET` | Secret key for JWT signing | *required* |
| `PORT` | API server port | `8080` |

## Optional Variables

| Variable | Description | Default |
|----------|-------------|---------|
| `UPLOAD_PATH` | Directory for file uploads | `./uploads` |
| `BASE_URL` | Base URL for generated links | `http://localhost:8080` |
| `GIN_MODE` | Gin framework mode | `debug` |

## Example `.env` File

```env
# Database
CASSANDRA_HOST=localhost
CASSANDRA_KEYSPACE=geoloc

# Security
JWT_SECRET=your-super-secret-key-here

# Server
PORT=8080
BASE_URL=http://localhost:8080

# Upload
UPLOAD_PATH=./uploads

# Production
# GIN_MODE=release
```

## Production Recommendations

1. **JWT_SECRET**: Use a strong, randomly generated secret (32+ characters)
2. **GIN_MODE**: Set to `release` for production
3. **CASSANDRA_HOST**: Use your production Cassandra cluster address
4. **BASE_URL**: Set to your production domain (e.g., `https://api.yourapp.com`)
