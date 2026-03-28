-- Add backup eligibility and state flags to webauthn credentials.
-- The go-webauthn library validates BackupEligible consistency on login,
-- so these must be stored during registration and loaded during login.
ALTER TABLE webauthn_credentials ADD COLUMN backup_eligible BOOLEAN NOT NULL DEFAULT false;
ALTER TABLE webauthn_credentials ADD COLUMN backup_state BOOLEAN NOT NULL DEFAULT false;
