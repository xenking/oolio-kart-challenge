#!/usr/bin/env bash
set -euo pipefail

BASE_URL="https://orderfoodonline-files.s3.ap-southeast-2.amazonaws.com"
DATA_DIR="${DATA_DIR:-data}"

mkdir -p "$DATA_DIR"

for i in 1 2 3; do
    FILE="$DATA_DIR/couponbase${i}.gz"
    [ -f "$FILE" ] && echo "Exists: $FILE" && continue
    echo "Downloading couponbase${i}.gz..."
    curl -fSL -o "$FILE" "$BASE_URL/couponbase${i}.gz"
done

echo "Done. All coupon files downloaded to $DATA_DIR/"
