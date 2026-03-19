package db

import (
	"context"
	"fmt"
	"time"

	"github.com/raphi011/know/internal/models"
	"github.com/surrealdb/surrealdb.go"
)

// CreateDeviceCode stores a new device authorization request.
func (c *Client) CreateDeviceCode(ctx context.Context, deviceCode, userCode string, expiresAt time.Time) error {
	defer c.logOp(ctx, "device_code.create", time.Now())
	sql := `CREATE device_code SET device_code = $device_code, user_code = $user_code, expires_at = $expires_at`
	if _, err := surrealdb.Query[any](ctx, c.DB(), sql, map[string]any{
		"device_code": deviceCode,
		"user_code":   userCode,
		"expires_at":  expiresAt,
	}); err != nil {
		return fmt.Errorf("create device code: %w", err)
	}
	return nil
}

// GetDeviceCodeByUserCode looks up a pending device authorization by user code.
func (c *Client) GetDeviceCodeByUserCode(ctx context.Context, userCode string) (*models.DeviceCode, error) {
	defer c.logOp(ctx, "device_code.get_by_user_code", time.Now())
	sql := `SELECT * FROM device_code WHERE user_code = $user_code LIMIT 1`
	results, err := surrealdb.Query[[]models.DeviceCode](ctx, c.DB(), sql, map[string]any{
		"user_code": userCode,
	})
	if err != nil {
		return nil, fmt.Errorf("get device code by user code: %w", err)
	}
	return firstResultOpt(results), nil
}

// GetDeviceCodeByCode looks up a device authorization by device code (for polling).
func (c *Client) GetDeviceCodeByCode(ctx context.Context, deviceCode string) (*models.DeviceCode, error) {
	defer c.logOp(ctx, "device_code.get_by_code", time.Now())
	sql := `SELECT * FROM device_code WHERE device_code = $device_code LIMIT 1`
	results, err := surrealdb.Query[[]models.DeviceCode](ctx, c.DB(), sql, map[string]any{
		"device_code": deviceCode,
	})
	if err != nil {
		return nil, fmt.Errorf("get device code by code: %w", err)
	}
	return firstResultOpt(results), nil
}

// ApproveDeviceCode marks a device code as approved and links the user + token.
// The rawToken is stored temporarily in the raw_token field so the polling CLI
// can retrieve it. The device code record is deleted after successful poll.
// Returns an error if the user code does not match any record.
func (c *Client) ApproveDeviceCode(ctx context.Context, userCode string, userID string, rawToken string) error {
	defer c.logOp(ctx, "device_code.approve", time.Now())
	sql := `UPDATE device_code SET approved = true, user = type::record("user", $user_id), raw_token = $raw_token WHERE user_code = $user_code`
	results, err := surrealdb.Query[[]models.DeviceCode](ctx, c.DB(), sql, map[string]any{
		"user_code": userCode,
		"user_id":   bareID("user", userID),
		"raw_token": rawToken,
	})
	if err != nil {
		return fmt.Errorf("approve device code: %w", err)
	}
	if firstResultOpt(results) == nil {
		return fmt.Errorf("approve device code: no matching device code for user_code %q", userCode)
	}
	return nil
}

// DeleteDeviceCode removes a device code by its device code string.
func (c *Client) DeleteDeviceCode(ctx context.Context, deviceCode string) error {
	defer c.logOp(ctx, "device_code.delete", time.Now())
	sql := `DELETE device_code WHERE device_code = $device_code`
	if _, err := surrealdb.Query[any](ctx, c.DB(), sql, map[string]any{
		"device_code": deviceCode,
	}); err != nil {
		return fmt.Errorf("delete device code: %w", err)
	}
	return nil
}

// DeleteExpiredDeviceCodes removes expired device codes.
func (c *Client) DeleteExpiredDeviceCodes(ctx context.Context) error {
	defer c.logOp(ctx, "device_code.delete_expired", time.Now())
	sql := `DELETE device_code WHERE expires_at < time::now()`
	if _, err := surrealdb.Query[any](ctx, c.DB(), sql, nil); err != nil {
		return fmt.Errorf("delete expired device codes: %w", err)
	}
	return nil
}
