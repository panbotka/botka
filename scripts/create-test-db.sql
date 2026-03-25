-- Create the test database for handler integration tests.
-- Run: docker exec shared-postgres psql -U postgres -f /path/to/create-test-db.sql
-- Or:  psql -h localhost -U postgres -f scripts/create-test-db.sql
CREATE DATABASE botka_test OWNER botka;
