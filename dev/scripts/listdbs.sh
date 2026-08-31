#!/bin/bash
# Lists all databases on the PostgreSQL server.

export PGHOSTADDR=127.0.0.1
export PGPORT=7654
export PGUSER=postgres

PGPASSWORD=theeaglefliesatmidnight psql -l
