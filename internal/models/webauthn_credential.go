package models

import "time"

// WebAuthnCredential stores a registered WebAuthn passkey for a user.
type WebAuthnCredential struct {
	ID           int64     `gorm:"primaryKey;autoIncrement" json:"id"`
	UserID       int64     `gorm:"not null;index" json:"user_id"`
	CredentialID []byte    `gorm:"type:bytea;uniqueIndex;not null" json:"-"`
	PublicKey    []byte    `gorm:"type:bytea;not null" json:"-"`
	AAGUID       []byte    `gorm:"type:bytea" json:"-"`
	SignCount    uint32    `gorm:"not null;default:0" json:"-"`
	Name         string    `gorm:"not null;default:'Passkey'" json:"name"`
	CreatedAt    time.Time `gorm:"autoCreateTime" json:"created_at"`
}

// TableName returns the database table name for the WebAuthnCredential model.
func (WebAuthnCredential) TableName() string {
	return "webauthn_credentials"
}
