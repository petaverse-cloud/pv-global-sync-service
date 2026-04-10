# Global Sync Service

Cross-region data synchronization, global index, and feed generation service for WigoWago V2.

## Tech Stack

- **Language**: Go 1.22+
- **Message Queue**: Apache RocketMQ
- **Database**: PostgreSQL (Regional + Global Index)
- **Cache**: Redis Stack
- **Deployment**: Docker + Kubernetes (AKS)

## Architecture

This service is part of the WigoWago V2 distributed architecture, enabling GDPR-compliant cross-region data sharing between EU and NA deployment regions.

See [PROJECT_PLAN.md](docs/PROJECT_PLAN.md) for detailed project plan and [wigowago-v2-distributed-architecture.md](https://github.com/petaverse-cloud/pv-wigowago-api/blob/main/docs/) for the overall system design.

## Quick Start

### Prerequisites

- Go 1.22+
- PostgreSQL 16+ (two instances: regional + global index)
- Redis 7+
- Apache RocketMQ

### Development

```bash
# Install dependencies
make init

# Run tests
make test

# Build
make build

# Run (set environment variables first)
cp .env.sample .env
# Edit .env with your values
make run
```

### Docker

```bash
make docker-build
make docker-run
```

## Project Structure

```
cmd/server/          # Application entry point
internal/            # Private application code
  config/            # Configuration management
  server/            # HTTP server setup
  health/            # Health check endpoints
  model/             # Data models
  consumer/          # RocketMQ consumers
  producer/          # RocketMQ producers
  handler/           # HTTP request handlers
  service/           # Business logic
pkg/                 # Shared libraries
  rocketmq/          # RocketMQ client wrapper
  postgres/          # PostgreSQL client wrapper
  redis/             # Redis client wrapper
  logger/            # Logging utilities
deployments/         # K8s/Helm deployment configs
docs/                # Project documentation
scripts/             # Operational scripts
```

## License

Internal use only.
