package models

import "time"

// Setting stores a server-side configuration key-value pair.
type Setting struct {
	Key       string    `gorm:"column:key;primaryKey;size:100" json:"key"`
	Value     string    `gorm:"column:value;not null" json:"value"`
	UpdatedAt time.Time `gorm:"column:updated_at;autoUpdateTime" json:"updated_at"`
}

// TableName returns the database table name for the Setting model.
func (Setting) TableName() string {
	return "app_settings"
}
