package database

import (
	"crypto/rand"
	"fmt"
	"log/slog"
	"math/big"
	"os"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"

	"botka/internal/models"
)

const (
	defaultUsername = "botka"
	passwordLength  = 20
	credentialsFile = "/tmp/botka-initial-credentials.txt"
	passwordCharset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789!@#$%^&*"
)

// SeedInitialUser creates the initial admin user if no users exist.
// Returns nil if a user already exists. On first creation, prints
// credentials to stdout and writes them to a temp file.
func SeedInitialUser(db *gorm.DB) error {
	var count int64
	if err := db.Model(&models.User{}).Count(&count).Error; err != nil {
		return fmt.Errorf("count users: %w", err)
	}
	if count > 0 {
		return nil
	}

	password, err := generatePassword(passwordLength)
	if err != nil {
		return fmt.Errorf("generate password: %w", err)
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("hash password: %w", err)
	}

	user := models.User{
		Username:     defaultUsername,
		PasswordHash: string(hash),
	}
	if err := db.Create(&user).Error; err != nil {
		return fmt.Errorf("create user: %w", err)
	}

	msg := fmt.Sprintf("[auth] Created initial user: %s / %s", defaultUsername, password)
	slog.Info(msg)

	fileContent := fmt.Sprintf("Username: %s\nPassword: %s\n", defaultUsername, password)
	if err := os.WriteFile(credentialsFile, []byte(fileContent), 0600); err != nil {
		slog.Warn("failed to write credentials file", "path", credentialsFile, "error", err)
	} else {
		slog.Info("credentials written to file", "path", credentialsFile)
	}

	return nil
}

func generatePassword(length int) (string, error) {
	charset := []rune(passwordCharset)
	result := make([]rune, length)
	for i := range result {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(charset))))
		if err != nil {
			return "", err
		}
		result[i] = charset[n.Int64()]
	}
	return string(result), nil
}
