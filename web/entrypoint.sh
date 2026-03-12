#!/bin/sh
set -e

# Replace the build-time placeholder with the runtime VITE_API_URL value
if [ -n "$VITE_API_URL" ]; then
  find /usr/share/nginx/html/assets -name '*.js' -exec \
    sed -i "s|__API_URL_PLACEHOLDER__|${VITE_API_URL}|g" {} +
  echo "Injected VITE_API_URL=${VITE_API_URL}"
else
  echo "WARNING: VITE_API_URL not set, API calls will fail"
fi

exec "$@"
