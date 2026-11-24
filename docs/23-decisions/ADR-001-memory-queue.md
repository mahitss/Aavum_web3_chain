# ADR-001: Keep the job queue in memory

## Context

The orchestrator needs a list of jobs for the command center and for `verify-e2e.sh`.

## Decision

Store jobs in process memory. Do not add Postgres or Redis for the demo.

## Why

Settlement that matters is already on Aevum. A database would add backup, migration, and secret work without changing the escrow rules.

## Consequence

A crash forgets job ids. Receipts that were mined are still on the explorer. The refund button can only see task ids the current process still has.

## Reversal

Add a store later if more than one orchestrator must share the queue. The contract interface does not change.
