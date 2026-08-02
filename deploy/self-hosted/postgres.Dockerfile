FROM postgres:16-alpine

# Ship pg_cron-friendly extensions in the image. Enabling pg_cron itself
# requires shared_preload_libraries at runtime — deferred to a later task.
# pg_trgm, pgcrypto, and related extensions are also available after this step.
RUN apk add --no-cache postgresql16-contrib
