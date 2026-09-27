# DevTrackr API Documentation

Base URL: `http://localhost:<port>`

## Response Conventions

- Success responses are JSON unless the route performs a redirect.
- Error responses use this shape:

```json
{
  "error": "human readable message"
}
```

- Dates use `YYYY-MM-DD`.
- Digest times use `HH:MM` in 24-hour format.
- Nullable SQL fields may serialize as objects such as:

```json
{
  "String": "value",
  "Valid": true
}
```

## Public Routes

### GET /

Returns a welcome message.

Request sample:

```bash
curl http://localhost:8080/
```

Response sample:

```json
{
  "message": "Welcome to DevTrackr API!"
}
```

### GET /health

Returns a basic health status.

Request sample:

```bash
curl http://localhost:8080/health
```

Response sample:

```json
{
  "status": "ok"
}
```

## Authentication Routes

### GET /auth/github/login

Starts GitHub OAuth. This route sets a `github_oauth_state` cookie and redirects to GitHub.

Requested scopes are `read:user`, `user:email` and `repo`. The `repo` scope lets the collector see commits in the user's private repositories. Users who authorised before `repo` was added must log in with GitHub again to grant it and refresh their stored token.

Request sample:

```bash
curl -i http://localhost:8080/auth/github/login
```

Response sample:

```http
HTTP/1.1 307 Temporary Redirect
Set-Cookie: github_oauth_state=<state>; Path=/; HttpOnly; SameSite=Lax; Max-Age=300
Location: https://github.com/login/oauth/authorize?...&state=<state>
```

### GET /auth/github/callback

Handles the GitHub OAuth callback. Requires `code` and `state` query parameters and a matching `github_oauth_state` cookie.

Request sample:

```bash
curl -i "http://localhost:8080/auth/github/callback?code=abc123&state=xyz456"
```

Response sample when the frontend callback URL is configured:

```http
HTTP/1.1 302 Found
Set-Cookie: github_oauth_state=; Path=/; HttpOnly; SameSite=Lax; Max-Age=-1
Set-Cookie: devtrackr_auth=<token>; Path=/; HttpOnly; SameSite=Lax; Expires=<time>
Location: https://frontend.example.com/auth/callback?token=<token>&user_id=<user-id>&email=user@example.com&is_new_user=false&password_set=true
```

Response sample when no frontend callback URL is configured:

```json
{
  "token": "<token>",
  "user": {
    "id": "550e8400-e29b-41d4-a716-446655440000",
    "email": "user@example.com",
    "created_at": "2026-06-25T12:00:00Z",
    "updated_at": "2026-06-25T12:00:00Z"
  },
  "is_new_user": true,
  "password_set": false
}
```

### POST /auth/register

Registers a user with email and password.

Request body:

```json
{
  "email": "user@example.com",
  "password": "password123"
}
```

Response sample:

```json
{
  "token": "<token>",
  "user": {
    "id": "550e8400-e29b-41d4-a716-446655440000",
    "email": "user@example.com",
    "created_at": "2026-06-25T12:00:00Z",
    "updated_at": "2026-06-25T12:00:00Z"
  },
  "is_new_user": true,
  "password_set": true
}
```

Common error responses:

```json
{ "error": "email already taken" }
```

```json
{ "error": "password must be at least 8 characters" }
```

### POST /auth/login

Logs in with either `identifier` or `username` plus password.

Request body:

```json
{
  "identifier": "user@example.com",
  "password": "password123"
}
```

Alternative request body:

```json
{
  "username": "devuser",
  "password": "password123"
}
```

Response sample:

```json
{
  "token": "<token>",
  "user": {
    "id": "550e8400-e29b-41d4-a716-446655440000",
    "email": "user@example.com",
    "created_at": "2026-06-25T12:00:00Z",
    "updated_at": "2026-06-25T12:00:00Z"
  },
  "is_new_user": false,
  "password_set": true
}
```

Common error responses:

```json
{ "error": "invalid credentials" }
```

### POST /auth/password/setup/request

Requests a verification code for setting up a password on an existing account.

Request body:

```json
{
  "email": "user@example.com"
}
```

Response sample:

```json
{
  "status": "code_sent",
  "message": "If the account exists, a verification code has been sent."
}
```

### POST /auth/password/setup/confirm

Confirms the setup code and sets the password. A username can optionally be provided.

Request body:

```json
{
  "email": "user@example.com",
  "code": "123456",
  "new_password": "newpassword123",
  "username": "devuser"
}
```

Response sample:

```json
{
  "token": "<token>",
  "user": {
    "id": "550e8400-e29b-41d4-a716-446655440000",
    "email": "user@example.com",
    "username": {
      "String": "devuser",
      "Valid": true
    },
    "created_at": "2026-06-25T12:00:00Z",
    "updated_at": "2026-06-25T12:00:00Z"
  },
  "is_new_user": false,
  "password_set": true
}
```

Common error responses:

```json
{ "error": "invalid verification code" }
```

```json
{ "error": "verification code expired" }
```

### POST /auth/password/forgot/request

Requests a password reset verification code.

Request body:

```json
{
  "email": "user@example.com"
}
```

Response sample:

```json
{
  "status": "code_sent",
  "message": "If the account exists, a verification code has been sent."
}
```

### POST /auth/password/forgot/confirm

Confirms a password reset code and sets a new password.

Request body:

```json
{
  "email": "user@example.com",
  "code": "123456",
  "new_password": "newpassword123"
}
```

Response sample:

```json
{
  "token": "<token>",
  "user": {
    "id": "550e8400-e29b-41d4-a716-446655440000",
    "email": "user@example.com",
    "created_at": "2026-06-25T12:00:00Z",
    "updated_at": "2026-06-25T12:00:00Z"
  },
  "is_new_user": false,
  "password_set": true
}
```

## User Routes

### POST /users

Creates a user profile.

Request body:

```json
{
  "username": "devuser",
  "email": "user@example.com",
  "email_frequency": "daily",
  "timezone": "Asia/Kolkata",
  "digest_time": "23:30",
  "email_opt_in": true,
  "profile_public": false
}
```

Notes:

- `email` is required.
- `timezone` defaults to `Asia/Kolkata`.
- `digest_time` defaults to `23:30`.

Response sample:

```json
{
  "id": "550e8400-e29b-41d4-a716-446655440000",
  "username": {
    "String": "devuser",
    "Valid": true
  },
  "email": "user@example.com",
  "email_frequency": "daily",
  "timezone": "Asia/Kolkata",
  "digest_time": "23:30",
  "email_opt_in": true,
  "profile_public": false,
  "created_at": "2026-06-25T12:00:00Z",
  "updated_at": "2026-06-25T12:00:00Z"
}
```

### GET /users

Returns all users.

Request sample:

```bash
curl http://localhost:8080/users
```

Response sample:

```json
[
  {
    "id": "550e8400-e29b-41d4-a716-446655440000",
    "email": "user@example.com",
    "created_at": "2026-06-25T12:00:00Z",
    "updated_at": "2026-06-25T12:00:00Z"
  }
]
```

### GET /users/by-email?email=<email>

Returns a user by email.

Request sample:

```bash
curl "http://localhost:8080/users/by-email?email=user@example.com"
```

Response sample:

```json
{
  "id": "550e8400-e29b-41d4-a716-446655440000",
  "email": "user@example.com",
  "created_at": "2026-06-25T12:00:00Z",
  "updated_at": "2026-06-25T12:00:00Z"
}
```

### GET /users/by-username?username=<username>

Returns a user by username.

Request sample:

```bash
curl "http://localhost:8080/users/by-username?username=devuser"
```

Response sample:

```json
{
  "id": "550e8400-e29b-41d4-a716-446655440000",
  "username": {
    "String": "devuser",
    "Valid": true
  },
  "email": "user@example.com",
  "created_at": "2026-06-25T12:00:00Z",
  "updated_at": "2026-06-25T12:00:00Z"
}
```

### PATCH /users/{id}/email-opt-in

Updates the email opt-in flag.

Request body:

```json
{
  "email_opt_in": true
}
```

Response sample:

```json
{
  "status": "updated"
}
```

### PATCH /users/{id}/profile-public

Updates profile visibility.

Request body:

```json
{
  "profile_public": true
}
```

Response sample:

```json
{
  "status": "updated"
}
```

### PATCH /users/{id}/username

Updates a user's username.

Request body:

```json
{
  "username": "newname"
}
```

Response sample:

```json
{
  "status": "updated"
}
```

Common error responses:

```json
{ "error": "username already taken" }
```

```json
{ "error": "username is required" }
```

### PATCH /users/{id}/digest-time

Updates the digest send time.

Request body:

```json
{
  "digest_time": "21:45"
}
```

Response sample:

```json
{
  "status": "updated"
}
```

### POST /users/{id}/aggregate

Runs aggregation for one date. If `date` is omitted, the current UTC day is used.

Request sample:

```bash
curl -X POST "http://localhost:8080/users/550e8400-e29b-41d4-a716-446655440000/aggregate?date=2026-06-24"
```

Response sample:

```json
{
  "status": "aggregation_complete",
  "user_id": "550e8400-e29b-41d4-a716-446655440000",
  "date": "2026-06-24",
  "metric": {
    "id": "11111111-2222-3333-4444-555555555555",
    "user_id": "550e8400-e29b-41d4-a716-446655440000",
    "metric_date": "2026-06-24T00:00:00Z",
    "github_commits": 3,
    "lc_easy_solved": 1,
    "lc_medium_solved": 0,
    "lc_hard_solved": 0,
    "cf_problems_solved": 2,
    "streak_days": 4,
    "computed_at": "2026-06-25T12:00:00Z"
  }
}
```

### POST /users/{id}/aggregate/range

Runs aggregation for a date range.

Request sample:

```bash
curl -X POST "http://localhost:8080/users/550e8400-e29b-41d4-a716-446655440000/aggregate/range?start=2026-06-20&end=2026-06-24"
```

Response sample:

```json
{
  "status": "bulk_aggregation_complete",
  "user_id": "550e8400-e29b-41d4-a716-446655440000",
  "start": "2026-06-20",
  "end": "2026-06-24",
  "days_processed": 5,
  "days_failed": 0,
  "results": [
    {
      "date": "2026-06-20",
      "metric": {
        "id": "11111111-2222-3333-4444-555555555555",
        "user_id": "550e8400-e29b-41d4-a716-446655440000",
        "metric_date": "2026-06-20T00:00:00Z",
        "github_commits": 1,
        "lc_easy_solved": 0,
        "lc_medium_solved": 1,
        "lc_hard_solved": 0,
        "cf_problems_solved": 0,
        "streak_days": 1,
        "computed_at": "2026-06-25T12:00:00Z"
      }
    }
  ],
  "failed_dates": []
}
```

### POST /users/{id}/send-digest

Triggers a digest for one user. If `date` is omitted, the current IST day is used.

Request sample:

```bash
curl -X POST "http://localhost:8080/users/550e8400-e29b-41d4-a716-446655440000/send-digest?date=2026-06-25"
```

Response sample:

```json
{
  "status": "digest_processed",
  "user_id": "550e8400-e29b-41d4-a716-446655440000",
  "date": "2026-06-25"
}
```

### GET /users/{id}/integrations/active

Returns all active integrations for a user.

Request sample:

```bash
curl http://localhost:8080/users/550e8400-e29b-41d4-a716-446655440000/integrations/active
```

Response sample:

```json
[
  {
    "id": "66666666-7777-8888-9999-000000000000",
    "user_id": "550e8400-e29b-41d4-a716-446655440000",
    "platform": "github",
    "handle": "devuser",
    "is_active": true,
    "created_at": "2026-06-25T12:00:00Z"
  }
]
```

### GET /users/{id}/activities?date=<date>

Returns activities for a user on one date.

Request sample:

```bash
curl "http://localhost:8080/users/550e8400-e29b-41d4-a716-446655440000/activities?date=2026-06-24"
```

Response sample:

```json
{
  "user_id": "550e8400-e29b-41d4-a716-446655440000",
  "date": "2026-06-24",
  "activities": [
    {
      "id": "77777777-8888-9999-aaaa-bbbbbbbbbbbb",
      "user_id": "550e8400-e29b-41d4-a716-446655440000",
      "integration_id": "66666666-7777-8888-9999-000000000000",
      "platform": "github",
      "activity_date": "2026-06-24T00:00:00Z",
      "activity_type": "commit",
      "metadata": {
        "repo": "devtrackr/backend",
        "count": 3
      },
      "fetched_at": "2026-06-25T12:00:00Z"
    }
  ]
}
```

### GET /users/{id}/metrics?date=<date>

Returns the daily metric for one date.

Request sample:

```bash
curl "http://localhost:8080/users/550e8400-e29b-41d4-a716-446655440000/metrics?date=2026-06-24"
```

Response sample:

```json
{
  "id": "11111111-2222-3333-4444-555555555555",
  "user_id": "550e8400-e29b-41d4-a716-446655440000",
  "metric_date": "2026-06-24T00:00:00Z",
  "github_commits": 3,
  "lc_easy_solved": 1,
  "lc_medium_solved": 0,
  "lc_hard_solved": 0,
  "cf_problems_solved": 2,
  "streak_days": 4,
  "computed_at": "2026-06-25T12:00:00Z"
}
```

### GET /users/{id}/metrics/range?start=<date>&end=<date>

Returns metrics in a date range.

Request sample:

```bash
curl "http://localhost:8080/users/550e8400-e29b-41d4-a716-446655440000/metrics/range?start=2026-06-20&end=2026-06-24"
```

Response sample:

```json
[
  {
    "id": "11111111-2222-3333-4444-555555555555",
    "user_id": "550e8400-e29b-41d4-a716-446655440000",
    "metric_date": "2026-06-20T00:00:00Z",
    "github_commits": 1,
    "lc_easy_solved": 0,
    "lc_medium_solved": 1,
    "lc_hard_solved": 0,
    "cf_problems_solved": 0,
    "streak_days": 1,
    "computed_at": "2026-06-25T12:00:00Z"
  }
]
```

### GET /users/{id}/heatmap?start=<date>&end=<date>

Returns a heatmap series across a date range.

Request sample:

```bash
curl "http://localhost:8080/users/550e8400-e29b-41d4-a716-446655440000/heatmap?start=2026-06-20&end=2026-06-24"
```

Response sample:

```json
{
  "user_id": "550e8400-e29b-41d4-a716-446655440000",
  "start": "2026-06-20",
  "end": "2026-06-24",
  "days": [
    {
      "date": "2026-06-20",
      "total_contributions": 2,
      "github_commits": 1,
      "lc_easy_solved": 0,
      "lc_medium_solved": 1,
      "lc_hard_solved": 0,
      "cf_problems_solved": 0
    },
    {
      "date": "2026-06-21",
      "total_contributions": 0,
      "github_commits": 0,
      "lc_easy_solved": 0,
      "lc_medium_solved": 0,
      "lc_hard_solved": 0,
      "cf_problems_solved": 0
    }
  ]
}
```

### DELETE /users/{id}/activities&dmetrics

Deletes a user's activities and metrics.

Request sample:

```bash
curl -X DELETE http://localhost:8080/users/550e8400-e29b-41d4-a716-446655440000/activities\&dmetrics
```

Response sample:

```json
{
  "status": "deleted",
  "user_id": "550e8400-e29b-41d4-a716-446655440000",
  "activities_deleted": 12,
  "metrics_deleted": 12
}
```

## Integration Routes

### POST /integrations

Creates or updates an integration.

Request body:

```json
{
  "user_id": "550e8400-e29b-41d4-a716-446655440000",
  "platform": "github",
  "handle": "devuser",
  "access_token": "ghp_example",
  "is_active": true
}
```

Notes:

- `user_id`, `platform`, and `handle` are required.
- `is_active` is optional and defaults to `true`.
- `access_token` is optional. When omitted or empty, any token already stored for that user/platform is kept. When provided it replaces the stored token.
- Tokens are encrypted with AES-256-GCM before they are written to the database, using the key in the `INTEGRATION_TOKEN_KEY` environment variable. The server refuses to start without it. Generate one with `go run ./cmd/migrate -generate-token-key`, and run `go run ./cmd/migrate -reencrypt-tokens` once to encrypt tokens stored before this change.
- Tokens are never returned by any endpoint. Responses expose `has_token` instead.

Response sample:

```json
{
  "id": "66666666-7777-8888-9999-000000000000",
  "user_id": "550e8400-e29b-41d4-a716-446655440000",
  "platform": "github",
  "handle": "devuser",
  "has_token": true,
  "is_active": true,
  "created_at": "2026-06-25T12:00:00Z"
}
```

### DELETE /integrations/{id}

Deactivates an integration.

Request sample:

```bash
curl -X DELETE http://localhost:8080/integrations/66666666-7777-8888-9999-000000000000
```

Response sample:

```json
{
  "status": "deactivated"
}
```

## Job Routes

### POST /jobs/aggregate

Runs the daily aggregation job. If `date` is omitted, yesterday UTC is used.

Request sample:

```bash
curl -X POST "http://localhost:8080/jobs/aggregate?date=2026-06-24"
```

Response sample:

```json
{
  "status": "aggregation_complete",
  "date": "2026-06-24"
}
```

### POST /jobs/aggregate/backfill-2026

Runs aggregation for every day from 2026-01-01 through today in IST.

Request sample:

```bash
curl -X POST http://localhost:8080/jobs/aggregate/backfill-2026
```

Response sample:

```json
{
  "status": "backfill_complete",
  "start": "2026-01-01",
  "end": "2026-06-25",
  "days_processed": 176,
  "days_failed": 0,
  "failed_dates": []
}
```

If some dates fail, the status becomes `backfill_partial` and `failed_dates` contains the failures.

### POST /jobs/nightly

Runs the nightly job. If `date` is omitted, yesterday UTC is used.

Request sample:

```bash
curl -X POST "http://localhost:8080/jobs/nightly?date=2026-06-24"
```

Response sample:

```json
{
  "status": "nightly_complete",
  "date": "2026-06-24"
}
```

### POST /jobs/digest

Processes digests for all users for a given date. If `date` is omitted, the current IST day is used.

Request sample:

```bash
curl -X POST "http://localhost:8080/jobs/digest?date=2026-06-25"
```

Response sample:

```json
{
  "status": "digest_all_processed",
  "date": "2026-06-25"
}
```

### POST /jobs/digest/{id}

Processes a digest for a single user and date. If `date` is omitted, the current IST day is used.

Request sample:

```bash
curl -X POST "http://localhost:8080/jobs/digest/550e8400-e29b-41d4-a716-446655440000?date=2026-06-25"
```

Response sample:

```json
{
  "status": "digest_user_processed",
  "date": "2026-06-25",
  "user_id": "550e8400-e29b-41d4-a716-446655440000"
}
```

## Common Error Examples

```json
{ "error": "invalid request body" }
```

```json
{ "error": "user id is required" }
```

```json
{ "error": "date must be YYYY-MM-DD" }
```

```json
{ "error": "digest_time must be HH:MM (24-hour format)" }
```

```json
{ "error": "email and password are required" }
```