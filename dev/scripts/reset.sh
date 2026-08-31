#!/bin/bash
# Resets the development environment by deleting secrets and databases created during
# integration testing with the kind cluster.

ROLE=demo_endeavor
DATABASE=demo_endeavor
SECRET_NAME=demo-endeavor
NAMESPACE=endeavor

# Delete the secret if it exists.
kubectl delete secret -n $NAMESPACE $SECRET_NAME

## Connect as admin and delete the database and role if they exist.
export PGHOSTADDR=127.0.0.1
export PGPORT=7654
export PGUSER=postgres

PGPASSWORD=theeaglefliesatmidnight psql -c "DROP DATABASE IF EXISTS \"$DATABASE\""
PGPASSWORD=theeaglefliesatmidnight psql -c "DROP ROLE IF EXISTS \"$ROLE\""