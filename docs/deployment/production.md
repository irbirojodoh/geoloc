# Production Deployment

Considerations for deploying Geoloc to production.

## Checklist

- [ ] Set `GIN_MODE=release`
- [ ] Use strong `JWT_SECRET` (32+ characters)
- [ ] Configure production Cassandra cluster
- [ ] Set up HTTPS with TLS certificates
- [ ] Configure CORS for production domains
- [ ] Restrict file upload paths
- [ ] Set up monitoring and logging
- [ ] Configure rate limiting

## Environment

```env
GIN_MODE=release
JWT_SECRET=<strong-random-secret>
CASSANDRA_HOST=<cassandra-cluster-host>
CASSANDRA_KEYSPACE=geoloc
BASE_URL=https://api.yourdomain.com
PORT=8080
```

## Cassandra Production

### Recommended Setup
- 3+ node cluster for redundancy
- Replication factor of 3
- NetworkTopologyStrategy for multi-DC

### Keyspace Configuration
```cql
CREATE KEYSPACE geoloc WITH replication = {
  'class': 'NetworkTopologyStrategy',
  'dc1': 3
};
```

## HTTPS / TLS

Use a reverse proxy (nginx, Traefik) or cloud load balancer for TLS termination:

```nginx
server {
    listen 443 ssl;
    server_name api.yourdomain.com;
    
    ssl_certificate /path/to/cert.pem;
    ssl_certificate_key /path/to/key.pem;
    
    location / {
        proxy_pass http://localhost:8080;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
    }
}
```

## CORS Configuration

Update `cmd/api/main.go` to restrict origins:

```go
router.Use(cors.New(cors.Config{
    AllowOrigins:     []string{"https://yourdomain.com"},
    AllowMethods:     []string{"GET", "POST", "PUT", "DELETE"},
    AllowHeaders:     []string{"Origin", "Content-Type", "Authorization"},
    AllowCredentials: true,
}))
```

## File Storage

For production, consider:
- **Cloud storage** (S3, GCS, Cloudflare R2)
- **CDN** for serving uploads
- Modify upload handlers to use cloud SDK

## Monitoring

Recommended integrations:
- **Logging**: Structured JSON logs, ship to Loki/CloudWatch
- **Metrics**: Prometheus + Grafana
- **APM**: Datadog, New Relic, or Sentry

## Scaling

### API Servers
- Stateless design allows horizontal scaling
- Use load balancer to distribute traffic

### Cassandra
- Add nodes to cluster for capacity
- Geohash partitioning distributes load geographically

## Security

1. **Never expose** Cassandra port (9042) publicly
2. **Rate limit** by IP and/or user ID
3. **Validate** all user input
4. **Sanitize** file uploads
5. **Rotate** JWT secrets periodically
