# ADR-002: Four processes, no cluster

## Context

The product is a contract, a worker, an orchestrator, and a command center.

## Decision

Ship those four. Run them with Docker Compose on one host. Do not introduce Kubernetes or extra microservices.

## Why

Each process has a different key and a different job. Splitting further would add network failure modes the demo does not need.

## Consequence

Healthchecks order the start. One host is the availability boundary.

## Reversal

Split hosts only after a funded deployment exists and the single host is the measured bottleneck. No such measurement exists.
