package models

import "testing"

func TestSetting_TableName(t *testing.T) {
	s := Setting{}
	if s.TableName() != "app_settings" {
		t.Errorf("expected table name 'app_settings', got %q", s.TableName())
	}
}
