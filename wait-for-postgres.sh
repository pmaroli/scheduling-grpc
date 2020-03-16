#!/bin/sh

set -e

cmd="$@"

# until PGPASSWORD=$PG_PASSWORD psql -h $PG_HOST -p $PG_PORT -U $PG_USER --dbname=$PG_DB -c '\q'; do
#   >&2 echo "Postgres is unavailable - sleeping"
#   sleep 1
# done

# TODO: make an actual script for this
# psql is not available on the container that this runs on
sleep 5

>&2 echo "Postgres is up - starting server"
exec $cmd