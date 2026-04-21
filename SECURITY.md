# Security Policy

## Reporting a Vulnerability

If you discover a security vulnerability in the NinerLog API, please report it responsibly.

**Do NOT open a public GitHub issue for security vulnerabilities.**

Instead, please email **hej@ninerlog.com** with:

- A description of the vulnerability
- Steps to reproduce the issue
- The potential impact
- Any suggested fixes (if available)

We will acknowledge receipt within 48 hours and aim to provide a fix within 7 days for critical issues.

## Scope

This policy covers the `ninerlog-api` repository, including:

- REST API endpoints and request handling
- Authentication and authorization (JWT, TOTP 2FA)
- Password hashing (bcrypt)
- Database access and SQL injection prevention
- SMTP email handling
- File upload/export processing

## Security Practices

- **Authentication**: JWT with short-lived access tokens (15 min) and refresh tokens (7 days)
- **2FA**: TOTP-based two-factor authentication
- **Password Hashing**: bcrypt with appropriate cost factor
- **Database**: Parameterized queries, PostgreSQL with SSL in production
- **Input Validation**: OpenAPI schema validation on all endpoints
- **Rate Limiting**: Applied to authentication endpoints
- **Dependencies**: Automated updates via Dependabot

## Disclosure Policy

We follow a coordinated disclosure process. After a fix is released, we will publicly acknowledge the reporter (unless they prefer to remain anonymous).
