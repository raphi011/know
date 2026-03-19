package db

import (
	"context"
	"testing"
	"time"

	"github.com/raphi011/know/internal/models"
)

func TestCreateAndGetDeviceCode(t *testing.T) {
	ctx := context.Background()

	userCode := "TESTABCD"
	deviceCode := "deadbeef01234567890abcdef01234567890abcdef01234567890abcdef012345"
	expiresAt := time.Now().Add(15 * time.Minute)

	err := testDB.CreateDeviceCode(ctx, deviceCode, userCode, expiresAt)
	if err != nil {
		t.Fatalf("CreateDeviceCode failed: %v", err)
	}

	// Get by user code
	dc, err := testDB.GetDeviceCodeByUserCode(ctx, userCode)
	if err != nil {
		t.Fatalf("GetDeviceCodeByUserCode failed: %v", err)
	}
	if dc == nil {
		t.Fatal("GetDeviceCodeByUserCode returned nil")
	}
	if dc.UserCode != userCode {
		t.Errorf("expected user_code %q, got %q", userCode, dc.UserCode)
	}
	if dc.DeviceCode != deviceCode {
		t.Errorf("expected device_code %q, got %q", deviceCode, dc.DeviceCode)
	}
	if dc.Approved {
		t.Error("expected approved=false initially")
	}

	// Get by device code
	dc2, err := testDB.GetDeviceCodeByCode(ctx, deviceCode)
	if err != nil {
		t.Fatalf("GetDeviceCodeByCode failed: %v", err)
	}
	if dc2 == nil {
		t.Fatal("GetDeviceCodeByCode returned nil")
	}
	if dc2.UserCode != userCode {
		t.Errorf("expected user_code %q, got %q", userCode, dc2.UserCode)
	}

	// Not found
	notFound, err := testDB.GetDeviceCodeByUserCode(ctx, "NONEXIST")
	if err != nil {
		t.Fatalf("GetDeviceCodeByUserCode nonexistent error: %v", err)
	}
	if notFound != nil {
		t.Error("expected nil for nonexistent user code")
	}
}

func TestApproveDeviceCode(t *testing.T) {
	ctx := context.Background()
	user := createTestUser(t, ctx)
	userID := models.MustRecordIDString(user.ID)

	userCode := "APPROVAB"
	deviceCode := "approve1234567890abcdef01234567890abcdef01234567890abcdef012345ab"
	expiresAt := time.Now().Add(15 * time.Minute)

	err := testDB.CreateDeviceCode(ctx, deviceCode, userCode, expiresAt)
	if err != nil {
		t.Fatalf("CreateDeviceCode failed: %v", err)
	}

	rawToken := "kh_faketoken1234567890abcdef"
	err = testDB.ApproveDeviceCode(ctx, userCode, userID, rawToken)
	if err != nil {
		t.Fatalf("ApproveDeviceCode failed: %v", err)
	}

	dc, err := testDB.GetDeviceCodeByCode(ctx, deviceCode)
	if err != nil {
		t.Fatalf("GetDeviceCodeByCode failed: %v", err)
	}
	if dc == nil {
		t.Fatal("GetDeviceCodeByCode returned nil after approve")
	}
	if !dc.Approved {
		t.Error("expected approved=true after ApproveDeviceCode")
	}
	if dc.RawToken == nil || *dc.RawToken != rawToken {
		t.Errorf("expected raw_token %q, got %v", rawToken, dc.RawToken)
	}
	if dc.User == nil {
		t.Error("expected user to be set after approve")
	}
}

func TestApproveDeviceCodeNotFound(t *testing.T) {
	ctx := context.Background()
	user := createTestUser(t, ctx)
	userID := models.MustRecordIDString(user.ID)

	err := testDB.ApproveDeviceCode(ctx, "NONEXIST", userID, "kh_faketoken")
	if err == nil {
		t.Fatal("ApproveDeviceCode should return error for nonexistent user code")
	}
}

func TestDeleteDeviceCode(t *testing.T) {
	ctx := context.Background()

	userCode := "DELETEAB"
	deviceCode := "delete01234567890abcdef01234567890abcdef01234567890abcdef0123456a"
	expiresAt := time.Now().Add(15 * time.Minute)

	err := testDB.CreateDeviceCode(ctx, deviceCode, userCode, expiresAt)
	if err != nil {
		t.Fatalf("CreateDeviceCode failed: %v", err)
	}

	err = testDB.DeleteDeviceCode(ctx, deviceCode)
	if err != nil {
		t.Fatalf("DeleteDeviceCode failed: %v", err)
	}

	dc, err := testDB.GetDeviceCodeByCode(ctx, deviceCode)
	if err != nil {
		t.Fatalf("GetDeviceCodeByCode after delete failed: %v", err)
	}
	if dc != nil {
		t.Error("expected nil after delete")
	}
}

func TestDeleteExpiredDeviceCodes(t *testing.T) {
	ctx := context.Background()

	// Create an already-expired device code
	userCode := "EXPIREAB"
	deviceCode := "expire01234567890abcdef01234567890abcdef01234567890abcdef0123456a"
	expiresAt := time.Now().Add(-1 * time.Minute) // already expired

	err := testDB.CreateDeviceCode(ctx, deviceCode, userCode, expiresAt)
	if err != nil {
		t.Fatalf("CreateDeviceCode (expired) failed: %v", err)
	}

	// Create a valid (non-expired) device code
	userCode2 := "VALIDABC"
	deviceCode2 := "valid012345678900abcdef01234567890abcdef01234567890abcdef01234567"
	expiresAt2 := time.Now().Add(15 * time.Minute)

	err = testDB.CreateDeviceCode(ctx, deviceCode2, userCode2, expiresAt2)
	if err != nil {
		t.Fatalf("CreateDeviceCode (valid) failed: %v", err)
	}

	err = testDB.DeleteExpiredDeviceCodes(ctx)
	if err != nil {
		t.Fatalf("DeleteExpiredDeviceCodes failed: %v", err)
	}

	// Expired one should be gone
	dc, err := testDB.GetDeviceCodeByCode(ctx, deviceCode)
	if err != nil {
		t.Fatalf("GetDeviceCodeByCode expired after cleanup: %v", err)
	}
	if dc != nil {
		t.Error("expired device code should be deleted")
	}

	// Valid one should remain
	dc2, err := testDB.GetDeviceCodeByCode(ctx, deviceCode2)
	if err != nil {
		t.Fatalf("GetDeviceCodeByCode valid after cleanup: %v", err)
	}
	if dc2 == nil {
		t.Error("valid device code should still exist")
	}
}
