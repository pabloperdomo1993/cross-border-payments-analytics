# Cross-Border Payments Analytics — Architecture

## 1. Overview

Cross-Border Payments Analytics is an event-driven platform designed to
ingest, process, and analyze international payment transactions.

The system demonstrates a production-oriented architecture using:

- Go
- React + TypeScript
- Apache Kafka
- MariaDB
- ClickHouse
- Prometheus
- Grafana
- Docker
- Docker Compose

The entire platform is designed to run locally using Docker Compose.

A developer should only need Git and Docker to run the complete system.

---

## 2. Problem Statement

Companies operating cross-border payment platforms process transactions
across multiple countries, currencies, payment providers, and payment
corridors.

Operational databases are optimized for transactional workloads but are
not ideal for analytical queries over large volumes of historical payment
data.

This platform separates:

- Transaction ingestion
- Payment processing
- Event distribution
- Operational persistence
- Analytical persistence
- Analytics queries
- Observability

The system receives international payment transactions and produces
analytics across dimensions such as:

- Source country
- Destination country
- Source currency
- Destination currency
- Payment corridor
- Payment provider
- Transaction status
- Time period

---

## 3. Architecture Goals

The architecture is designed around the following principles:

- Event-driven communication
- Clear service boundaries
- Asynchronous payment processing
- Concurrent processing using Go
- Separation of OLTP and OLAP workloads
- Horizontal scalability
- Idempotent event processing
- Fault tolerance
- Observability by default
- Local reproducibility using Docker
- Graceful shutdown
- Automated testing

---

## 4. High-Level Architecture

```text
                           ┌─────────────────────┐
                           │       React         │
                           │    TypeScript       │
                           │      :5173          │
                           └─────────┬───────────┘
                                     │
                                     │ REST / JSON
                                     ▼
                           ┌─────────────────────┐
                           │  Payments Service   │
                           │        Go           │
                           │       :8080         │
                           └─────────┬───────────┘
                                     │
                          ┌──────────┴──────────┐
                          │                     │
                          ▼                     ▼
                     ┌─────────┐            ┌─────────┐
                     │ MariaDB │            │  Kafka  │
                     │  OLTP   │            │  KRaft  │
                     │  :3306  │            │  :9092  │
                     └─────────┘            └────┬────┘
                                                │
                                      payments.created
                                                │
                                                ▼
                                   ┌─────────────────────┐
                                   │ Payment Processor   │
                                   │        Go           │
                                   │                     │
                                   │    Worker Pool      │
                                   │    Goroutines       │
                                   └──────────┬──────────┘
                                              │
                                              │
                                  payments.processed
                                              │
                                              ▼
                                          ┌───────┐
                                          │ Kafka │
                                          └───┬───┘
                                              │
                                              ▼
                                   ┌─────────────────────┐
                                   │ Analytics Service   │
                                   │        Go           │
                                   │       :8082         │
                                   └──────────┬──────────┘
                                              │
                                              ▼
                                       ┌────────────┐
                                       │ ClickHouse │
                                       │    OLAP    │
                                       │   :8123    │
                                       └─────┬──────┘
                                             │
                                             │ analytics
                                             ▼
                                           React