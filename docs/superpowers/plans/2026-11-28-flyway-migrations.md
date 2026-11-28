# Flyway Migrations Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [x]`) syntax for tracking.

**Goal:** Apply the shared PostgreSQL schema once per release before service rollout.

**Architecture:** A release-specific Flyway image runs in a bounded Kubernetes Job. GitHub Actions waits for it before syncing ArgoCD or restarting changed services.

**Tech Stack:** Flyway CLI, PostgreSQL, Kubernetes, External Secrets, GitHub Actions, ArgoCD.

**Spec:** `docs/superpowers/specs/2026-11-28-flyway-migrations-design.md`

## Global Constraints

Use `db/postgres/migrations/` unchanged, with `public.flyway_schema_history` and no automatic baseline. Use distinct Vault paths per namespace. Do not run against stage/prod from this session.

## Review Focus

First run with missing credentials; failed Job vs timeout; repeat migrations; later push while a release is active; new ArgoCD sync must target the migrated commit.

## Task 1: One-shot runner

- [x] Test `run.sh stage <image> <run-key>` against fake kubectl: asserts create/wait ordering, nonzero exit for missing Secret readiness, failed Job, and timeout.
- [x] Add `iac/images/s1-flyway-migrations/Dockerfile`, Flyway config, Job/ExternalSecret templates and `run.sh` under `iac/jobs/s1-flyway-migrations/`. Runner exits zero only on Job Complete. Use release/run identity for unique Jobs; use bounded waits and attach failure logs.
- [x] Run mocked tests. In Actions build and test that the image applies all checked-in SQL to temporary PostgreSQL and that a second run succeeds with unchanged history.

## Task 2: Deployment order and setup

- [x] Test workflow graph: release depends on builds; migration precedes ArgoCD, then restart; no deploy on PR; environment concurrency and exact revision.
- [x] Modify `entrypoint.yml`, `_build-module.yml` and build/Argo actions as needed. Retain per-service change filtering on PRs and prod-only behavior; build all push images in the target environment; transfer the successful built Deployment names to the release job. Release consumes migration image digest.
- [x] Document Vault keys, permissions, image pull secret, and recovery in the migration module README. Run syntax, graph, runner and diff checks.

Ruling: build every target-environment service on push — queued runs may be replaced by newer pushes, so a last-push diff can miss unpublished changes — more build time in exchange for a complete release.

Local Docker/PostgreSQL smoke execution requires Docker; the check is wired into CI before publication.

Ruling: promote service digests to runtime tags only after Flyway — existing pods always pull by tag, independent of explicit rollout — adds a registry promotion step while keeping the existing ArgoCD image convention.

Ruling: require the current branch tip or latest version tag and disable Flyway's future-migration ignore — serial execution alone does not prevent a stale retry — release operators must retry the current ref, not deploy older tags.

Ruling: prevent ApplicationSet garbage collection from cascading outside the release pipeline — removing an app directory could otherwise delete workloads during a failed migration — retire applications explicitly and apply controller policy during bootstrap.
