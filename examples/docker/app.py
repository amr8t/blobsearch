#!/usr/bin/env python3
"""
Example application that generates structured JSON logs to stdout.
Compatible with OpenTelemetry logging format for modern web applications.
"""

import json
import os
import random
import sys
import time
from datetime import datetime

# Web app log patterns
WEBAPP_PATTERNS = [
    {"level": "info", "template": "Request processed successfully"},
    {"level": "info", "template": "User {user_id} logged in from {ip}"},
    {
        "level": "info",
        "template": "API request: {method} {endpoint} completed in {duration}ms",
    },
    {"level": "info", "template": "Cache hit for key {cache_key}"},
    {"level": "info", "template": "Database query completed in {duration}ms"},
    {"level": "debug", "template": "Processing request for endpoint {endpoint}"},
    {"level": "debug", "template": "Validating request parameters"},
    {"level": "debug", "template": "Cache lookup for key {cache_key}"},
    {"level": "warn", "template": "Slow query detected: {duration}ms for {endpoint}"},
    {"level": "warn", "template": "High memory usage: {percentage}% of limit"},
    {"level": "warn", "template": "Rate limit approaching for user {user_id}"},
    {"level": "warn", "template": "Cache miss rate above threshold: {percentage}%"},
    {
        "level": "warn",
        "template": "Queue depth for {queue} is {count} (threshold: {threshold})",
    },
    {"level": "error", "template": "Database connection failed: {error}"},
    {
        "level": "error",
        "template": "Failed to process payment for user {user_id}: {error}",
    },
    {
        "level": "error",
        "template": "Authentication failed for user {user_id} from {ip}",
    },
    {"level": "error", "template": "Request timeout after {duration}ms for {endpoint}"},
    {
        "level": "error",
        "template": "Failed to connect to {database}: connection refused",
    },
    {"level": "error", "template": "Validation error: {error}"},
    {"level": "error", "template": "Unexpected error in {endpoint}: {error}"},
]

SERVICES = [
    "api-gateway",
    "auth-service",
    "payment-service",
    "user-service",
    "notification-service",
    "order-service",
    "inventory-service",
]

ENDPOINTS = [
    "/api/v1/users",
    "/api/v1/orders",
    "/api/v1/products",
    "/api/v1/auth/login",
    "/api/v1/auth/register",
    "/api/v1/payments",
    "/api/v1/inventory",
    "/api/v2/users",
    "/health",
    "/metrics",
]

HTTP_METHODS = ["GET", "POST", "PUT", "DELETE", "PATCH"]
STATUS_CODES = [200, 201, 204, 400, 401, 403, 404, 422, 500, 502, 503]
ERROR_CODES = [
    "ERR_DB_CONNECTION",
    "ERR_TIMEOUT",
    "ERR_VALIDATION",
    "ERR_AUTH_FAILED",
    "ERR_NOT_FOUND",
    "ERR_PAYMENT_FAILED",
]
ERROR_MESSAGES = [
    "connection timeout",
    "validation failed",
    "user not found",
    "permission denied",
    "invalid token",
    "database error",
]
WARNING_TYPES = ["performance", "security", "capacity", "deprecation"]
DATABASES = ["postgres-primary", "postgres-replica", "redis-cache", "mongodb-orders"]
QUEUES = ["email-queue", "notification-queue", "analytics-queue", "payment-queue"]
CACHE_KEYS = ["user", "session", "product", "inventory", "config"]


def generate_ip():
    """Generate a random IP address."""
    return f"{random.randint(1, 255)}.{random.randint(0, 255)}.{random.randint(0, 255)}.{random.randint(1, 255)}"


def generate_request_id():
    """Generate a random request ID."""
    return f"req_{''.join(random.choices('abcdef0123456789', k=16))}"


def generate_trace_id():
    """Generate a random trace ID."""
    return "".join(random.choices("abcdef0123456789", k=32))


def generate_span_id():
    """Generate a random span ID."""
    return "".join(random.choices("abcdef0123456789", k=16))


def generate_stack_trace():
    """Generate a simple stack trace."""
    traces = [
        "at handleRequest (app.js:145)",
        "at Database.query (db.js:89)",
        "at validateUser (auth.js:234)",
        "at processPayment (payment.js:456)",
        "at sendEmail (email.js:78)",
    ]
    num_lines = random.randint(2, 4)
    return " | ".join(random.sample(traces, min(num_lines, len(traces))))


def format_message(template):
    """Format a log message from a template."""
    replacements = {
        "{user_id}": f"user_{random.randint(1, 10000)}",
        "{endpoint}": random.choice(ENDPOINTS),
        "{method}": random.choice(HTTP_METHODS),
        "{status}": str(random.choice(STATUS_CODES)),
        "{duration}": str(random.randint(10, 5000)),
        "{error}": random.choice(ERROR_MESSAGES),
        "{ip}": generate_ip(),
        "{count}": str(random.randint(1, 1000)),
        "{threshold}": str(random.randint(50, 100)),
        "{database}": random.choice(DATABASES),
        "{queue}": random.choice(QUEUES),
        "{cache_key}": f"cache:{random.choice(CACHE_KEYS)}:{random.randint(1, 10000)}",
        "{percentage}": f"{random.uniform(0, 100):.2f}",
    }

    message = template
    for key, value in replacements.items():
        message = message.replace(key, value, 1)
    return message


def generate_log():
    """Generate a single structured web app log entry (JSON)."""
    pattern = random.choice(WEBAPP_PATTERNS)

    log_entry = {
        "timestamp": datetime.now().isoformat(),
        "level": pattern["level"],
        "service": random.choice(SERVICES),
        "message": format_message(pattern["template"]),
    }

    # Add additional structured fields based on log type
    if pattern["level"] == "error":
        log_entry["error_code"] = random.choice(ERROR_CODES)
        if random.random() < 0.6:
            log_entry["stack_trace"] = generate_stack_trace()
    elif pattern["level"] == "warn":
        log_entry["warning_type"] = random.choice(WARNING_TYPES)

    # Add request context for some logs
    if random.random() < 0.7:
        log_entry["request_id"] = generate_request_id()
        log_entry["user_id"] = f"user_{random.randint(1, 10000)}"
        log_entry["endpoint"] = random.choice(ENDPOINTS)
        log_entry["method"] = random.choice(HTTP_METHODS)
        log_entry["duration_ms"] = random.randint(10, 5000)
        log_entry["status_code"] = random.choice(STATUS_CODES)

    # Add trace context for distributed tracing
    if random.random() < 0.5:
        log_entry["trace_id"] = generate_trace_id()
        log_entry["span_id"] = generate_span_id()

    return json.dumps(log_entry)


def main():
    """Main function that continuously generates logs."""
    print("[App] Starting structured JSON log generator...", file=sys.stderr)
    print("[App] Generating logs to stdout", file=sys.stderr)

    counter = 0
    while True:
        try:
            log = generate_log()
            print(log, flush=True)
            counter += 1

            # Log progress to stderr (won't be captured by log forwarder)
            if counter % 100 == 0:
                print(f"[App] Generated {counter} log entries", file=sys.stderr)

            # Random sleep between 0.1 and 2 seconds to simulate realistic traffic
            time.sleep(random.uniform(0.1, 2.0))

        except KeyboardInterrupt:
            print(
                f"\n[App] Shutting down... Generated {counter} total logs",
                file=sys.stderr,
            )
            sys.exit(0)
        except Exception as e:
            print(f"[App] Error: {e}", file=sys.stderr)
            time.sleep(1)


if __name__ == "__main__":
    main()
