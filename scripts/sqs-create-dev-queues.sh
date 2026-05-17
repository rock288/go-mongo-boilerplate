#!/usr/bin/env bash
# Create the dev queue trio (main, retry, dlq) on LocalStack, wiring a
# native redrive policy as a SECOND layer of safety. App-level retry queue
# handles the normal flow; redrive policy catches crashed/stuck messages.
#
# Production: do NOT use this — manage queues via Terraform/CDK so the IaC is
# the source of truth and the worker only needs sqs:Send/Receive/Delete IAM.
#
# Usage:
#   make localstack-up        # start LocalStack first
#   make sqs-create-queues    # then create queues
#
# Env:
#   SQS_ENDPOINT (default http://localhost:4566)
#   AWS_REGION   (default us-east-1)
#   QUEUE_NAME   (default events) — base name; -retry / -dlq are appended

set -euo pipefail

ENDPOINT="${SQS_ENDPOINT:-http://localhost:4566}"
REGION="${AWS_REGION:-us-east-1}"
BASE="${QUEUE_NAME:-events}"

if ! command -v aws >/dev/null; then
  echo "error: awscli required — brew install awscli" >&2
  exit 1
fi

aws_cmd() {
  AWS_ACCESS_KEY_ID="${AWS_ACCESS_KEY_ID:-test}" \
  AWS_SECRET_ACCESS_KEY="${AWS_SECRET_ACCESS_KEY:-test}" \
  aws --endpoint-url="$ENDPOINT" --region="$REGION" "$@"
}

create() {
  local name="$1"
  local extra_args=("${@:2}")
  echo "→ create $name"
  aws_cmd sqs create-queue --queue-name "$name" "${extra_args[@]}" >/dev/null || \
    echo "  (already exists)"
}

create "${BASE}-dlq"
DLQ_ARN=$(aws_cmd sqs get-queue-attributes \
  --queue-url "${ENDPOINT}/000000000000/${BASE}-dlq" \
  --attribute-names QueueArn \
  --query 'Attributes.QueueArn' --output text)

REDRIVE=$(printf '{"deadLetterTargetArn":"%s","maxReceiveCount":"5"}' "$DLQ_ARN")
create "${BASE}-retry"
create "$BASE" --attributes "RedrivePolicy=$REDRIVE"

echo
echo "queues ready (region=$REGION endpoint=$ENDPOINT):"
echo "  ${ENDPOINT}/000000000000/${BASE}"
echo "  ${ENDPOINT}/000000000000/${BASE}-retry"
echo "  ${ENDPOINT}/000000000000/${BASE}-dlq"
echo
echo "set in your .env:"
echo "  APP_SQS__CONSUMER__QUEUE_URL=${ENDPOINT}/000000000000/${BASE}"
echo "  APP_SQS__ENDPOINT=${ENDPOINT}"
