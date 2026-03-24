-- Create the botka database and user on shared PostgreSQL
-- Run: psql -h localhost -U postgres -f scripts/create-db.sql

CREATE USER botka WITH PASSWORD 'botka';
CREATE DATABASE botka OWNER botka;
