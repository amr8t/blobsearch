#!/bin/sh
set -e

# Build the command with required flags
CMD="./ingestor -http -port=${HTTP_PORT}"

# Add S3 configuration
if [ -n "$ENDPOINT" ]; then
    CMD="$CMD -endpoint=$ENDPOINT"
fi

if [ -n "$ACCESS_KEY" ]; then
    CMD="$CMD -access-key=$ACCESS_KEY"
fi

if [ -n "$SECRET_KEY" ]; then
    CMD="$CMD -secret-key=$SECRET_KEY"
fi

if [ -n "$REGION" ]; then
    CMD="$CMD -region=$REGION"
fi

# Add standard flags
CMD="$CMD -bucket=$BUCKET"
CMD="$CMD -prefix=$PREFIX"
CMD="$CMD -batch-size=$BATCH_SIZE"
CMD="$CMD -compression=$COMPRESSION"
CMD="$CMD -with-timestamps=$WITH_TIMESTAMPS"
CMD="$CMD -deduplicate=$DEDUPLICATE"
CMD="$CMD -dedup-window=$DEDUP_WINDOW"
CMD="$CMD -auto-flush=$AUTO_FLUSH"
CMD="$CMD -auto-flush-interval=$AUTO_FLUSH_INTERVAL"

# Execute the command
exec $CMD
