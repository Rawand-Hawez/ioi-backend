# IOI High-Performance Backend Skeleton

A professional, self-hosted backend infrastructure designed for high-concurrency and sub-millisecond latency. This skeleton provides a domain-agnostic foundation that matches the developer experience of managed platforms while offering superior performance and absolute architectural control.

## Technical Architecture

This project utilizes a strictly curated stack to ensure maximum throughput and resource efficiency:

### Core API Layer
- **Go & Fiber v2**: Leverages the `fasthttp` engine for zero-allocation memory management and high-concurrency handling. Unlike Node.js or Python, the compiled Go binary eliminates runtime overhead and provides true multi-threaded execution.
- **pREST**: Provides instantaneous, highly optimized RESTful endpoints for standard CRUD operations directly from the PostgreSQL schema, significantly reducing boilerplate code.

### Data Persistence & Integrity
- **PostgreSQL 18**: The primary source of truth. Security is enforced natively through Row-Level Security (RLS) policies, ensuring a zero-trust architecture where authorization is handled at the database level.
- **pgx/v5**: A high-performance, native PostgreSQL driver for Go. It outperforms the standard `database/sql` package by utilizing PostgreSQL-specific features and binary protocols.
- **sqlc**: Generates type-safe Go code from raw SQL. This approach provides the safety of an ORM with the raw performance of handcrafted SQL queries, eliminating the "hidden" overhead and complex query generation of traditional ORMs.

### Infrastructure & State
- **DragonflyDB**: A multi-threaded, Redis-compatible in-memory data store. It is designed to scale vertically on multi-core machines, providing significantly higher throughput and lower tail latency than standard Redis.
- **Supabase GoTrue**: A standalone identity service that manages user authentication and issues cryptographically signed JWTs.
- **MinIO**: High-performance, S3-compatible object storage for handling media and document assets.

## Performance Gains Over Managed Solutions

By moving from a managed platform (e.g., Supabase) to this self-hosted architecture, several performance bottlenecks are eliminated:

1. **Network Latency**: In this architecture, the API gateway, the custom logic layer, and the database reside within the same high-speed Docker network. This reduces internal communication latency to sub-millisecond levels.
2. **Cold Starts**: Managed "Edge Functions" often suffer from execution pauses (cold starts). Our Go-based services run as persistent binaries, ensuring instant response times for every request.
3. **Optimized Caching**: The integration of DragonflyDB allows for aggressive caching of expensive queries and session state with multi-threaded performance that exceeds standard managed Redis offerings.
4. **Driver Efficiency**: By using `pgx` and `sqlc` instead of generic drivers or heavy ORMs, the application minimizes CPU cycles spent on reflection and data mapping during database interactions.

## Development Workflow

This repository provides a standardized implementation workflow for building high-performance features.

### Quick Start
1. Configure environment: `cp .env.example .env`
2. Initialize infrastructure: `docker compose up -d`
3. Generate DB bindings: `make db-gen`
4. Run development server: `make dev`

### Database Branching
Safely test migrations by cloning the primary database:
```bash
make db-branch-create BRANCH=feature-testing
make db-branch-switch BRANCH=feature-testing
# When finished:
make db-branch-switch BRANCH=app_database
make db-branch-drop BRANCH=feature-testing
```
