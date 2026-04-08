# E2E Test Regressions

Issues discovered by the e2e test suite running against the real API.
Tests are marked with `REGRESSION:` prefix and log the issue without failing.

## HIGH Severity

### 1. No password length validation on registration
- **Endpoint:** `POST /auth/register`
- **Test:** `TestAuthRegistration/REGRESSION:_register_with_short_password_should_return_400`
- **Expected:** 400 Bad Request for passwords shorter than 8 characters
- **Actual:** 201 Created — password `"short"` is accepted
- **Risk:** Users can set weak passwords, violating security policy

### 2. No required field validation on registration
- **Endpoint:** `POST /auth/register`
- **Test:** `TestAuthRegistration/REGRESSION:_register_with_missing_fields_should_return_400`
- **Expected:** 400 Bad Request when `password` is missing
- **Actual:** 201 Created — account created with empty password hash
- **Risk:** Accounts with no password can be created

## MEDIUM Severity

### 3. Empty name accepted on registration
- **Endpoint:** `POST /auth/register`
- **Test:** `TestAuthRegistration/REGRESSION:_register_with_empty_name_should_return_400`
- **Expected:** 400 Bad Request when `name` is empty string
- **Actual:** 201 Created — user created with `name: ""`
- **Risk:** Data quality issue, UI displays blank names

### 4. Email not normalized to lowercase
- **Endpoint:** `POST /auth/register`
- **Test:** `TestAuthRegistration/REGRESSION:_register_email_should_be_case_insensitive`
- **Expected:** 409 Conflict when registering `USER@X.COM` after `user@x.com`
- **Actual:** 201 Created — two separate accounts for the same email
- **Risk:** Duplicate accounts, login confusion, password reset issues

### 5. License creation with missing fields returns 500
- **Endpoint:** `POST /licenses`
- **Test:** `TestLicenseCRUD/REGRESSION:_missing_fields_should_return_400_not_500`
- **Expected:** 400 Bad Request with validation error details
- **Actual:** 500 Internal Server Error — `"Failed to create license"`
- **Risk:** Exposes internal errors to clients, poor developer experience

## LOW Severity

### 6. DELETE /users/me/data returns 200 instead of 204
- **Endpoint:** `DELETE /users/me/data`
- **Test:** `TestDeleteAllUserData/delete_data_keeps_account`
- **Expected:** 204 No Content (per OpenAPI spec)
- **Actual:** 200 OK with `{"message":"All user data deleted successfully"}`
- **Risk:** Minor OpenAPI compliance issue

### 7. Very long email (>255 chars) causes 500
- **Endpoint:** `POST /auth/register`
- **Test:** `TestEmailValidation/REGRESSION:_very_long_email_causes_500`
- **Expected:** 400 Bad Request with validation error
- **Actual:** 500 Internal Server Error (exceeds VARCHAR(255) column)
- **Risk:** Unhandled DB error exposed to client

### 8. Very long password (>72 bytes) causes 500
- **Endpoint:** `POST /auth/register`
- **Test:** `TestPasswordValidation/REGRESSION:_very_long_password_causes_500`
- **Expected:** 400 Bad Request or silent truncation to 72 bytes (bcrypt limit)
- **Actual:** 500 Internal Server Error
- **Risk:** Unhandled bcrypt limit error exposed to client

### 9. Credential accepts expiry date before issue date
- **Endpoint:** `POST /credentials`
- **Test:** `TestCredentialExpiryEdgeCases/expiry_before_issue_date`
- **Expected:** 400 Bad Request (expiry must be after issue)
- **Actual:** 201 Created with logically invalid dates
- **Risk:** Data integrity issue, confusing currency/expiry calculations

### 10. Email update to existing email causes 500
- **Endpoint:** `PATCH /users/me`
- **Test:** `TestEmailUpdateDuplicate/update_email_to_another_user_email_fails`
- **Expected:** 409 Conflict
- **Actual:** 500 Internal Server Error (`"Failed to update user"`)
- **Risk:** Unhandled unique constraint violation exposed to client
