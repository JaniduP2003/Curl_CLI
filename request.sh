#!/bin/bash

# Postman CLI - HTTP Request Handler
# This script handles actual HTTP requests using curl and formats the output with colors

# Color codes for terminal output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
MAGENTA='\033[0;35m'
CYAN='\033[0;36m'
WHITE='\033[1;37m'
GRAY='\033[0;90m'
NC='\033[0m' # No Color

# Parse arguments
METHOD="${1:-GET}"
URL="${2}"
HEADERS="${3}"
BODY="${4}"
BODY_TYPE=$(echo "${5}" | tr 'A-Z' 'a-z') # "raw", "none", or empty

# Validate inputs
if [ -z "$URL" ]; then
    echo -e "${RED}Error: URL is required${NC}"
    exit 1
fi

# Create temporary files for storing response
RESPONSE_HEADERS=$(mktemp)
RESPONSE_BODY=$(mktemp)
RESPONSE_CODE=$(mktemp)

# Cleanup temporary files on exit
trap "rm -f $RESPONSE_HEADERS $RESPONSE_BODY $RESPONSE_CODE" EXIT

# Detect/normalize Content-Type
has_content_type=false
ct_value=""
if [ -n "$HEADERS" ]; then
    while IFS= read -r header; do
        # trim leading spaces for matching
        trimmed=$(echo "$header" | sed -E 's/^\s+//')
        if [[ -n "$trimmed" && "$trimmed" =~ ^[Cc][Oo][Nn][Tt][Ee][Nn][Tt]-[Tt][Yy][Pp][Ee]: ]]; then
            has_content_type=true
            ct_value=$(echo "$trimmed" | cut -d: -f2- | tr -d '\r' | xargs)
        fi
    done <<< "$HEADERS"
fi

# Decide how to send the body
send_body=false
send_raw=false
if { [ "$METHOD" = "POST" ] || [ "$METHOD" = "PUT" ]; } && [ -n "$BODY" ]; then
    case "$BODY_TYPE" in
        raw)
            send_body=true
            send_raw=true
            # For raw, default to JSON if not specified
            if ! $has_content_type; then
                HEADERS=$(printf "%s\n%s" "$HEADERS" "Content-Type: application/json")
                has_content_type=true
                ct_value="application/json"
            fi
            ;;
        none)
            send_body=false
            ;;
        *)
            # Fallback heuristics when type not specified
            send_body=true
            if $has_content_type; then
                ct_lower=$(echo "$ct_value" | tr 'A-Z' 'a-z')
                if [[ "$ct_lower" != application/x-www-form-urlencoded* ]]; then
                    send_raw=true
                fi
            else
                # If body looks like JSON, set CT and use raw
                if [[ "$BODY" =~ ^[[:space:]]*\{ ]] || [[ "$BODY" =~ ^[[:space:]]*\[ ]]; then
                    HEADERS=$(printf "%s\n%s" "$HEADERS" "Content-Type: application/json")
                    has_content_type=true
                    ct_value="application/json"
                    send_raw=true
                fi
            fi
            ;;
    esac
fi

# Build curl command using array to avoid eval
CURL_ARGS=(
    -s
    -w '%{http_code}'
    -o "$RESPONSE_BODY"
    -D "$RESPONSE_HEADERS"
    -X "$METHOD"
)

# Add headers
if [ -n "$HEADERS" ]; then
    while IFS= read -r header; do
        if [ -n "$header" ]; then
            CURL_ARGS+=( -H "$header" )
        fi
    done <<< "$HEADERS"
fi

# Add body
if $send_body; then
    if $send_raw; then
        # Use --data-raw to send the body exactly without URL-encoding
        CURL_ARGS+=( --data-raw "$BODY" )
    else
        CURL_ARGS+=( --data "$BODY" )
    fi
fi

# Add URL
CURL_ARGS+=( "$URL" )

# Execute curl and capture status
HTTP_CODE=$(curl "${CURL_ARGS[@]}")
echo "$HTTP_CODE" > "$RESPONSE_CODE"

# Read the response code
STATUS_CODE=$(cat $RESPONSE_CODE)

# Determine status color based on code
if [ "$STATUS_CODE" -ge 200 ] && [ "$STATUS_CODE" -lt 300 ]; then
    STATUS_COLOR=$GREEN
elif [ "$STATUS_CODE" -ge 300 ] && [ "$STATUS_CODE" -lt 400 ]; then
    STATUS_COLOR=$YELLOW
elif [ "$STATUS_CODE" -ge 400 ] && [ "$STATUS_CODE" -lt 500 ]; then
    STATUS_COLOR=$RED
else
    STATUS_COLOR=$MAGENTA
fi

# Print formatted output
echo -e "${CYAN}╔════════════════════════════════════════════════════════════════╗${NC}"
echo -e "${CYAN}║${WHITE}                       HTTP RESPONSE                          ${CYAN}║${NC}"
echo -e "${CYAN}╚════════════════════════════════════════════════════════════════╝${NC}"
echo ""

# Display status line
echo -e "${WHITE}Status:${NC} ${STATUS_COLOR}${STATUS_CODE}${NC}"
echo ""

# Display response headers
echo -e "${CYAN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo -e "${YELLOW}Response Headers:${NC}"
echo -e "${CYAN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"

# Parse and display headers (skip HTTP status line and empty lines)
while IFS= read -r line; do
    # Skip empty lines and HTTP status line
    if [[ -n "$line" && ! "$line" =~ ^HTTP/ ]]; then
        # Split header name and value
        if [[ "$line" =~ ^([^:]+):(.+)$ ]]; then
            header_name="${BASH_REMATCH[1]}"
            header_value="${BASH_REMATCH[2]}"
            echo -e "${BLUE}${header_name}:${NC}${header_value}"
        fi
    fi
done < "$RESPONSE_HEADERS"

echo ""

# Display response body
echo -e "${CYAN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo -e "${YELLOW}Response Body:${NC}"
echo -e "${CYAN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"

# Check if response body is JSON and format it
if command -v jq &> /dev/null; then
    # Use jq to format JSON if available
    if jq -e . "$RESPONSE_BODY" &> /dev/null; then
        cat "$RESPONSE_BODY" | jq -C '.'
    else
        # Not valid JSON, display as-is
        cat "$RESPONSE_BODY"
    fi
else
    # jq not available, try basic JSON detection and indentation
    if grep -q "^{" "$RESPONSE_BODY" || grep -q "^\[" "$RESPONSE_BODY"; then
        # Looks like JSON, use python for formatting if available
        if command -v python3 &> /dev/null; then
            python3 -m json.tool < "$RESPONSE_BODY" 2>/dev/null || cat "$RESPONSE_BODY"
        elif command -v python &> /dev/null; then
            python -m json.tool < "$RESPONSE_BODY" 2>/dev/null || cat "$RESPONSE_BODY"
        else
            cat "$RESPONSE_BODY"
        fi
    else
        # Not JSON, display as-is
        cat "$RESPONSE_BODY"
    fi
fi

echo ""
echo -e "${CYAN}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo ""

# Display request summary
echo -e "${GRAY}Request: ${METHOD} ${URL}${NC}"
if [ -n "$HEADERS" ]; then
    echo -e "${GRAY}Headers: $(echo "$HEADERS" | wc -l | tr -d ' ') header(s) sent${NC}"
fi
if [ -n "$BODY" ]; then
    echo -e "${GRAY}Body: $(echo "$BODY" | wc -c | tr -d ' ') bytes sent${NC}"
fi

echo ""