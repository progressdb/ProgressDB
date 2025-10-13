# ProgressDB Service

ProgressDB is a simple HTTP API service designed for appending and retrieving messages grouped into threads. It uses PebbleDB for storage and supports optional field-level encryption using AES-256-GCM via an external KMS.

## Features

- Store and list messages by thread.
- Flexible message schema with custom JSON payloads.
- Optional encryption for full messages or specific fields.
- Prometheus metrics endpoint and health checks.
- Swagger/OpenAPI auto-generated documentation.
- Configurable via YAML file, environment variables, or flags.
- Built-in support for API keys, rate limiting, CORS, and TLS.

## Usage

- Start the service with a simple shell script or binary.
- Configure using a YAML file, environment variables, or flag overrides.
- Use API keys for both frontend (public) and backend (admin) operations.

## Data Model

Messages include fixed metadata (ID, thread, author, timestamp) and a flexible JSON body for custom data.
Threads are lightweight metadata groupings for messages.

## Security

API key authentication is required for all endpoints. TLS, CORS, IP allowlists, and rate limits are supported. Encryption uses an external key management service.

## Configuration

Supports `.env` loading, comprehensive YAML config, and various environment variables.
Fields, validation, encryption, authentication, and logging level are all easily configurable.

## Encryption

Messages can be encrypted in full or at selected fields, keeping essential metadata indexable and clear.

## Admin

Minimal admin endpoints are provided for operational tasks.