// SPDX-License-Identifier: AGPL-3.0-or-later
// Copyright (c) 2026 ShangBin Wang

package user

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"

	"rims-go/internal/modules/audit"
	"rims-go/internal/types"
)

type userHandlerAuditLogger struct {
	entries []audit.Entry
	err     error
}

func (l *userHandlerAuditLogger) Log(ctx context.Context, e audit.Entry) error {
	l.entries = append(l.entries, e)
	return l.err
}

type auditUserRepoStub struct {
	users map[uint]*User
	next  uint
}

func (r *auditUserRepoStub) Create(ctx context.Context, u *User) error {
	if r.next == 0 {
		r.next = 100
	}
	u.ID = r.next
	r.users[u.ID] = u
	return nil
}

func (r *auditUserRepoStub) GetByID(ctx context.Context, id uint) (*User, error) {
	u, ok := r.users[id]
	if !ok {
		return nil, gorm.ErrRecordNotFound
	}
	copy := *u
	return &copy, nil
}

func (r *auditUserRepoStub) GetByUsername(ctx context.Context, username string) (*User, error) {
	for _, u := range r.users {
		if u.Username == username {
			copy := *u
			return &copy, nil
		}
	}
	return nil, gorm.ErrRecordNotFound
}

func (r *auditUserRepoStub) GetAuthUser(ctx context.Context, userID uint) (uint, string, uint, string, int8, error) {
	u, err := r.GetByID(ctx, userID)
	if err != nil {
		return 0, "", 0, "", 0, err
	}
	roleCode := ""
	if u.Role != nil {
		roleCode = u.Role.Code
	}
	return u.ID, u.Username, u.RoleID, roleCode, u.Status, nil
}

func (r *auditUserRepoStub) List(ctx context.Context, page types.PageRequest) ([]User, int64, error) {
	return nil, 0, nil
}

func (r *auditUserRepoStub) Update(ctx context.Context, u *User) error {
	copy := *u
	r.users[u.ID] = &copy
	return nil
}

func (r *auditUserRepoStub) Delete(ctx context.Context, id uint) error {
	delete(r.users, id)
	return nil
}

type auditRoleRepoStub struct {
	roles          map[uint]*Role
	next           uint
	assignErr      error
	assignedRoleID uint
	assignedPerms  []uint
}

func (r *auditRoleRepoStub) Create(ctx context.Context, role *Role) error {
	if r.roles == nil {
		r.roles = make(map[uint]*Role)
	}
	if r.next == 0 {
		r.next = 100
	}
	role.ID = r.next
	r.next++
	copy := *role
	r.roles[role.ID] = &copy
	return nil
}
func (r *auditRoleRepoStub) GetByCode(ctx context.Context, code string) (*Role, error) {
	for _, role := range r.roles {
		if role.Code == code {
			copy := *role
			return &copy, nil
		}
	}
	return nil, gorm.ErrRecordNotFound
}
func (r *auditRoleRepoStub) GetByID(ctx context.Context, id uint) (*Role, error) {
	role, ok := r.roles[id]
	if !ok {
		return nil, gorm.ErrRecordNotFound
	}
	copy := *role
	return &copy, nil
}
func (r *auditRoleRepoStub) List(ctx context.Context) ([]Role, error) { return nil, nil }
func (r *auditRoleRepoStub) Update(ctx context.Context, role *Role) error {
	copy := *role
	r.roles[role.ID] = &copy
	return nil
}
func (r *auditRoleRepoStub) Delete(ctx context.Context, id uint) error {
	delete(r.roles, id)
	return nil
}
func (r *auditRoleRepoStub) AssignPermissions(ctx context.Context, roleID uint, permIDs []uint) error {
	r.assignedRoleID = roleID
	r.assignedPerms = append([]uint(nil), permIDs...)
	return r.assignErr
}
func (r *auditRoleRepoStub) ListPermissions(ctx context.Context) ([]Permission, error) {
	return nil, nil
}
func (r *auditRoleRepoStub) HasPermission(ctx context.Context, roleID uint, code string) (bool, error) {
	return false, nil
}

func TestUserHandlerAuditsCreateUpdateDeleteAndAssignPermissions(t *testing.T) {
	gin.SetMode(gin.TestMode)
	role := &Role{Code: "staff", Name: "Staff"}
	role.ID = 2
	userRepo := &auditUserRepoStub{
		users: map[uint]*User{
			7: {Username: "old", RealName: "Old", RoleID: 2, Status: 1, Role: role},
		},
		next: 8,
	}
	userRepo.users[7].ID = 7
	roleRepo := &auditRoleRepoStub{roles: map[uint]*Role{2: role}, next: 3}
	logger := &userHandlerAuditLogger{}
	handler := NewHandler(NewUserService(userRepo, roleRepo, nil), NewRoleService(roleRepo), logger)

	runUserAuditRequest(t, handler.CreateUser, http.MethodPost, "/users", "", "", `{"username":"newuser","password":"secret123","realName":"New User","roleId":2}`)
	runUserAuditRequest(t, handler.UpdateUser, http.MethodPut, "/users/7", "id", "7", `{"realName":"Updated"}`)
	runUserAuditRequest(t, handler.DeleteUser, http.MethodDelete, "/users/7", "id", "7", "")
	runUserAuditRequest(t, handler.CreateRole, http.MethodPost, "/roles", "", "", `{"code":"manager","name":"Manager","description":"Can manage stores"}`)
	runUserAuditRequest(t, handler.UpdateRole, http.MethodPut, "/roles/3", "id", "3", `{"name":"Store Manager"}`)
	runUserAuditRequest(t, handler.DeleteRole, http.MethodDelete, "/roles/3", "id", "3", "")
	runUserAuditRequest(t, handler.AssignPermissions, http.MethodPut, "/roles/2/permissions", "id", "2", `{"permissionIds":[3,4]}`)

	if len(logger.entries) != 7 {
		t.Fatalf("audit entries = %d, want 7", len(logger.entries))
	}
	assertUserAuditEntry(t, logger.entries[0], audit.ActionCreate, audit.ResourceUser, 8)
	assertUserAuditEntry(t, logger.entries[1], audit.ActionUpdate, audit.ResourceUser, 7)
	assertUserAuditEntry(t, logger.entries[2], audit.ActionDelete, audit.ResourceUser, 7)
	assertUserAuditEntry(t, logger.entries[3], audit.ActionCreate, audit.ResourceRole, 3)
	assertUserAuditEntry(t, logger.entries[4], audit.ActionUpdate, audit.ResourceRole, 3)
	assertUserAuditEntry(t, logger.entries[5], audit.ActionDelete, audit.ResourceRole, 3)
	assertUserAuditEntry(t, logger.entries[6], audit.ActionAssign, audit.ResourcePermission, 2)
	for i, entry := range logger.entries {
		if entry.Result != audit.ResultSuccess {
			t.Fatalf("entry %d result = %q, want %q", i, entry.Result, audit.ResultSuccess)
		}
	}
}

func TestUserHandlerAuditsAssignPermissionsAppErrorFailure(t *testing.T) {
	gin.SetMode(gin.TestMode)
	role := &Role{Code: "staff", Name: "Staff"}
	role.ID = 2
	roleRepo := &auditRoleRepoStub{
		roles:     map[uint]*Role{2: role},
		assignErr: types.ErrValidation("权限不存在"),
	}
	logger := &userHandlerAuditLogger{}
	handler := NewHandler(nil, NewRoleService(roleRepo), logger)

	_, rec := runUserAuditRequest(t, handler.AssignPermissions, http.MethodPut, "/roles/2/permissions", "id", "2", `{"permissionIds":[99]}`)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
	if len(logger.entries) != 1 {
		t.Fatalf("audit entries = %d, want 1", len(logger.entries))
	}
	got := logger.entries[0]
	if got.Result != audit.ResultFailure {
		t.Fatalf("result = %q, want %q", got.Result, audit.ResultFailure)
	}
	if got.ErrorCode != types.ErrCodeValidation || !strings.Contains(got.ErrorMsg, "权限不存在") {
		t.Fatalf("error = %d/%q, want validation 权限不存在", got.ErrorCode, got.ErrorMsg)
	}
	assertUserAuditEntry(t, got, audit.ActionAssign, audit.ResourcePermission, 2)
}

func TestUserHandlerIgnoresAuditLoggerErrorOnSuccessfulCreate(t *testing.T) {
	gin.SetMode(gin.TestMode)
	role := &Role{Code: "staff", Name: "Staff"}
	role.ID = 2
	userRepo := &auditUserRepoStub{users: map[uint]*User{}, next: 9}
	roleRepo := &auditRoleRepoStub{roles: map[uint]*Role{2: role}}
	logger := &userHandlerAuditLogger{err: errors.New("audit unavailable")}
	handler := NewHandler(NewUserService(userRepo, roleRepo, nil), NewRoleService(roleRepo), logger)

	_, rec := runUserAuditRequest(t, handler.CreateUser, http.MethodPost, "/users", "", "", `{"username":"newuser","password":"secret123","roleId":2}`)

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusCreated, rec.Body.String())
	}
	if len(logger.entries) != 1 {
		t.Fatalf("audit entries = %d, want 1", len(logger.entries))
	}
}

func TestUserHandlerAuditsPasswordChangeAndReset(t *testing.T) {
	gin.SetMode(gin.TestMode)
	role := &Role{Code: "staff", Name: "Staff"}
	role.ID = 2
	selfHash, err := bcrypt.GenerateFromPassword([]byte("old-secret"), bcrypt.DefaultCost)
	if err != nil {
		t.Fatalf("hash self password: %v", err)
	}
	targetHash, err := bcrypt.GenerateFromPassword([]byte("target-secret"), bcrypt.DefaultCost)
	if err != nil {
		t.Fatalf("hash target password: %v", err)
	}
	userRepo := &auditUserRepoStub{
		users: map[uint]*User{
			1: {Username: "admin", PasswordHash: string(selfHash), RoleID: 2, Status: 1, Role: role},
			9: {Username: "target", PasswordHash: string(targetHash), RoleID: 2, Status: 1, Role: role},
		},
	}
	userRepo.users[1].ID = 1
	userRepo.users[9].ID = 9
	roleRepo := &auditRoleRepoStub{roles: map[uint]*Role{2: role}}
	logger := &userHandlerAuditLogger{}
	handler := NewHandler(NewUserService(userRepo, roleRepo, nil), NewRoleService(roleRepo), logger)

	runUserAuditRequest(t, handler.ChangePassword, http.MethodPut, "/users/me/password", "", "", `{"oldPassword":"old-secret","newPassword":"new-secret"}`)
	runUserAuditRequest(t, handler.ResetPassword, http.MethodPut, "/users/9/password", "id", "9", `{"newPassword":"reset-secret"}`)

	if len(logger.entries) != 2 {
		t.Fatalf("audit entries = %d, want 2", len(logger.entries))
	}
	assertUserAuditEntry(t, logger.entries[0], audit.ActionUpdate, audit.ResourceUser, 1)
	if logger.entries[0].After["passwordChanged"] != true || logger.entries[0].After["userID"] != uint(1) {
		t.Fatalf("change password details = %#v, want passwordChanged/userID", logger.entries[0].After)
	}
	assertUserAuditEntry(t, logger.entries[1], audit.ActionUpdate, audit.ResourceUser, 9)
	if logger.entries[1].After["passwordReset"] != true || logger.entries[1].After["targetUserID"] != uint(9) {
		t.Fatalf("reset password details = %#v, want passwordReset/targetUserID", logger.entries[1].After)
	}
	for _, entry := range logger.entries {
		for _, forbiddenKey := range []string{"oldPassword", "newPassword", "password"} {
			if _, ok := entry.After[forbiddenKey]; ok {
				t.Fatalf("audit details include sensitive key %q: %#v", forbiddenKey, entry.After)
			}
		}
		details := fmt.Sprint(entry.After)
		for _, secret := range []string{"old-secret", "new-secret", "reset-secret", "target-secret"} {
			if strings.Contains(details, secret) {
				t.Fatalf("audit details leaked password value %q: %s", secret, details)
			}
		}
	}
}

func runUserAuditRequest(t *testing.T, fn gin.HandlerFunc, method, target, paramKey, paramValue, body string) (*gin.Context, *httptest.ResponseRecorder) {
	t.Helper()
	rec := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(rec)
	c.Request = httptest.NewRequest(method, target, strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set(types.CtxKeyUserID, uint(1))
	c.Set(types.CtxKeyUsername, "admin")
	c.Set(types.CtxKeyRoleCode, "admin")
	c.Set(types.CtxKeyTraceID, "trace-user-audit")
	if paramKey != "" {
		c.Params = gin.Params{{Key: paramKey, Value: paramValue}}
	}

	fn(c)
	if rec.Code < 200 || rec.Code >= 500 {
		t.Fatalf("%s %s status = %d; body=%s", method, target, rec.Code, rec.Body.String())
	}
	return c, rec
}

func assertUserAuditEntry(t *testing.T, got audit.Entry, action, resource string, resourceID uint) {
	t.Helper()
	if got.Action != action || got.Resource != resource {
		t.Fatalf("entry action/resource = %q/%q, want %q/%q", got.Action, got.Resource, action, resource)
	}
	if got.ResourceID == nil || *got.ResourceID != resourceID {
		t.Fatalf("entry resourceID = %v, want %d", got.ResourceID, resourceID)
	}
	if got.Actor.UserID != 1 || got.Actor.Username != "admin" || got.Actor.RoleCode != "admin" {
		t.Fatalf("actor = %#v, want admin actor", got.Actor)
	}
}
