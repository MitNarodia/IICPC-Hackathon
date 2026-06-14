# E2E Harness

The intended e2e flow is:

1. Start Postgres/TimescaleDB, Redpanda, Redis, MinIO, and a registry.
2. Run all Track 1 services with the environment variables from `CODING_PLAN.md`.
3. Upload each golden sample from `test/samples/{cpp,rust,go}`.
4. Assert every submission reaches `READY` and exposes an in-cluster endpoint.
5. Run the red-team corpus and assert blocked or contained behavior.

This harness is intentionally documented but not executed by default because it requires privileged container runtime features and a Kubernetes or single-node sandbox host.
