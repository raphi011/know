# User Stories

## Authentication

### AUTH-01: User signs in via SSO
**As a** user, **I want to** sign in with my organization's SSO provider **so that** I can access the application securely.

**Acceptance Criteria:**
- [ ] Login page shows "Sign in with SSO" button
- [ ] Clicking the button redirects to the OIDC provider
- [ ] After successful auth, user is redirected to /dashboard
- [ ] Unauthenticated users are redirected to /login

## Dashboard

### DASH-01: User views dashboard
**As a** signed-in user, **I want to** see a dashboard **so that** I can get an overview of my data.

**Acceptance Criteria:**
- [ ] Dashboard shows a welcome message with user's name
- [ ] Page is only accessible when authenticated

## Settings

### SETT-01: User changes theme and language
**As a** user, **I want to** change the app theme and language **so that** the app matches my preferences.

**Acceptance Criteria:**
- [ ] Settings page shows theme options (Light, Dark, Auto)
- [ ] Theme changes apply immediately without page reload
- [ ] Language can be switched between English and German
- [ ] Sign out button is available
